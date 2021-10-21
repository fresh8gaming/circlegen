package main

import (
	"bufio"
	"bytes"
	"embed"
	"io/ioutil"
	"log"
	"os"
	"text/template"

	"gopkg.in/yaml.v2"
)

//go:embed _template/config.yml
var configTemplate embed.FS

type Metadata struct {
	Name     string            `yaml:"name"`
	Team     string            `yaml:"team"`
	Domain   string            `yaml:"domain"`
	Services []MetadataService `yaml:"services"`
}

type MetadataService struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	CIEnabled bool   `yaml:"ciEnabled"`
}

func main() {
	metadataByte, err := ioutil.ReadFile(".metadata.yml")
	Fatal(err)

	var metadata Metadata
	err = yaml.Unmarshal(metadataByte, &metadata)
	Fatal(err)

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