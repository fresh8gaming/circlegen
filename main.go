package main

import (
	"bufio"
	"bytes"
	"embed"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/digitalocean/gta"
	yaml "gopkg.in/yaml.v3"
)

//go:embed _template/config.yml
var configTemplate embed.FS

type Metadata struct {
	Name                   string            `yaml:"name"`
	Staging                bool              `yaml:"staging"`
	Team                   string            `yaml:"team"`
	Domain                 string            `yaml:"domain"`
	WhitesourceEnabled     bool              `yaml:"whitesourceEnabled"`
	CDEnabled              bool              `yaml:"cdEnabled"`
	Services               []MetadataService `yaml:"services"`
	ArgoAppNamesProduction string            `yaml:"argoAppNamesProduction"`
	ArgoAppNamesStaging    string            `yaml:"argoAppNamesStaging"`

	ChangedServices []MetadataService
}

type MetadataService struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	CIEnabled bool   `yaml:"ciEnabled"`
}

//nolint:gocritic
func (m Metadata) HasGRPC() bool {
	for _, service := range m.Services {
		if service.Type == "http-grpc" {
			return true
		}
	}

	return false
}

func (m Metadata) NeedsApproval() bool {
	return m.Staging || !m.CDEnabled
}

func (ms MetadataService) NameUnderscored() string {
	return strings.ReplaceAll(ms.Name, "-", "_")
}

func main() {
	metadataByte, err := os.ReadFile(".metadata.yml")
	Fatal(err)

	var metadata Metadata
	err = yaml.Unmarshal(metadataByte, &metadata)
	Fatal(err)

	gitDifferOptions := []gta.GitDifferOption{
		gta.SetBaseBranch("trunk"),
	}

	options := []gta.Option{
		gta.SetDiffer(gta.NewGitDiffer(gitDifferOptions...)),
		// gta.SetPrefixes(parseStringSlice(*flagInclude)...),
		// gta.SetTags(tags...),
	}

	gt, err := gta.New(options...)
	if err != nil {
		log.Fatalf("can't prepare gta: %v", err)
	}

	packages, err := gt.ChangedPackages()
	if err != nil {
		log.Fatalf("can't list dirty packages: %v", err)
	}

	for _, changedPackage := range packages.AllChanges {
		pathParts := strings.Split(changedPackage.ImportPath, "/")

		if pathParts[1] == os.Getenv("CIRCLE_PROJECT_USERNAME") &&
			pathParts[2] == os.Getenv("CIRCLE_PROJECT_REPONAME") &&
			pathParts[3] == "cmd" {
			for _, service := range metadata.Services {
				if service.Name == pathParts[4] {
					metadata.ChangedServices = append(metadata.ChangedServices, service)
				}
			}
		}

	}

	f, err := os.Create(".circleci/generated-config.yml")
	Fatal(err)

	byteBuf := bytes.NewBuffer([]byte{})

	t, err := template.ParseFS(configTemplate, "_template/config.yml")
	Fatal(err)

	w := bufio.NewWriter(byteBuf)
	err = t.Execute(w, metadata)
	Fatal(err)

	err = w.Flush()
	Fatal(err)

	_, err = f.Write(byteBuf.Bytes())
	Fatal(err)
}

func Fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
