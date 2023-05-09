package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/digitalocean/gta"
	yaml "gopkg.in/yaml.v3"
)

const expectedPathParts = 5

//go:embed _template/config.yml
var configTemplate embed.FS

type Metadata struct {
	Name                   string            `yaml:"name"`
	Staging                bool              `yaml:"staging"`
	Team                   string            `yaml:"team"`
	Domain                 string            `yaml:"domain"`
	KubescoreEnabled       bool              `yaml:"kubescoreEnabled"`
	CDEnabled              bool              `yaml:"cdEnabled"`
	Services               []MetadataService `yaml:"services"`
	ArgoAppNamesProduction string            `yaml:"argoAppNamesProduction"`
	ArgoAppNamesStaging    string            `yaml:"argoAppNamesStaging"`
	Deploy                 Deploy            `yaml:"deploy"`
	GoVersion              string            `yaml:"goVersion,omitempty"`
	AlpineVersion          string            `yaml:"alpineVersion,omitempty"`
	TZDataVersion          string            `yaml:"tzDataVersion,omitempty"`
	CaCertVersion          string            `yaml:"caCertVersion,omitempty"`

	DisableWhitesource bool `yaml:"disableWhitesource"`

	ChangedServices []MetadataService
}

type Deploy struct {
	Platform string `yaml:"platform"`
	Product  string `yaml:"product"`
}

type MetadataService struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	CIEnabled  bool   `yaml:"ciEnabled"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
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

//nolint:gocritic
func (m Metadata) NeedsApproval() bool {
	return m.Staging || !m.CDEnabled
}

//nolint:gocritic
func (m Metadata) OverrideGoVersion() string {
	out := ""
	if m.GoVersion != "" {
		out = fmt.Sprintf(" --build-arg GO_VERSION=%s", m.GoVersion)
	}
	return out
}

//nolint:gocritic
func (m Metadata) OverrideAlpineVersions() string {
	out := ""
	if m.AlpineVersion != "" {
		out = fmt.Sprintf(" --build-arg ALPINE_VERSION=%s", m.AlpineVersion)
	}
	return out
}

//nolint:gocritic
func (m Metadata) OverrideAlpinePackagesVersions() string {
	out := ""
	if m.TZDataVersion != "" {
		out = fmt.Sprintf(" --build-arg TZDATA_VERSION=%s", m.TZDataVersion)
	}
	if m.CaCertVersion != "" {
		out = fmt.Sprintf(" --build-arg CA_CERTIFICATE_VERSION=%s", m.CaCertVersion)
	}
	return out
}

//nolint:gocritic
func (m Metadata) ArgOverrides() string {
	return m.OverrideGoVersion() + m.OverrideAlpineVersions() + m.OverrideAlpinePackagesVersions()
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

	if os.Getenv("CIRCLE_BRANCH") == "" {
		log.Fatal("CIRCLE_BRANCH envvar must be set")
	}

	// Compare to last commit on current branch
	gitDifferOptions := []gta.GitDifferOption{
		gta.SetBaseBranch(os.Getenv("CIRCLE_BRANCH") + "~1"),
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

		// branch if there are no changes to specific services ie only vendored dependencies
		if len(pathParts) < expectedPathParts {
			str := "a service, the go.mod and go.sum or another root level file may have changed"
			log.Printf("skipping no specific code changes to %s %s", pathParts, str)
			continue
		}

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
	if metadata.GoVersion == "" {
		cmd := "go mod edit -json | jq -r .Go"
		out, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			Fatal(err)
		}

		metadata.GoVersion = string(out)
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
