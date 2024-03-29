package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry/bosh-cli/release/manifest"
	yaml "gopkg.in/yaml.v2"
)

type Release struct {
	filename string
	tgz      *tar.Reader
}

type Manifest struct {
	bytes  []byte
	header *tar.Header
}

func main() {
	log.SetFlags(log.Lshortfile)

	if len(os.Args) < 3 {
		log.Fatalln("usage: bosh-merge <release> <release>...")
	}

	gz := gzip.NewWriter(os.Stdout)
	output := tar.NewWriter(gz)
	defer gz.Close()
	defer output.Close()

	var releases []Release
	for _, r := range os.Args[1:] {
		f, err := os.Open(r)
		if err != nil {
			log.Fatalln(err)
		}
		gzf, err := gzip.NewReader(f)
		if err != nil {
			log.Fatalln(err)
		}
		releases = append(releases, Release{filename: r, tgz: tar.NewReader(gzf)})
	}
	var manifests []Manifest
	uniqueEntries := map[string]string{}
	for _, r := range releases {
		for {
			header, err := r.tgz.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatalln(err)
			}
			if strings.HasSuffix(header.Name, "release.MF") {
				bs, err := ioutil.ReadAll(r.tgz)
				if err != nil {
					log.Fatalf("Error reading manifest in %v: %v", r.filename, err)
				}
				manifests = append(manifests, Manifest{bytes: bs, header: header})
			} else {
				if old, found := uniqueEntries[header.Name]; found {
					if header.Typeflag != tar.TypeDir {
						log.Fatalf("Found duplicated file %q present in both %v and %v", header.Name, old, r.filename)
					}
				}
				uniqueEntries[header.Name] = r.filename
				if err := output.WriteHeader(header); err != nil {
					log.Fatalf("Error writing tar header: %v", err)
				}
				if _, err := io.Copy(output, r.tgz); err != nil {
					log.Fatalf("Error writing to output stream: %v", err)
				}
			}
		}
	}

	var final manifest.Manifest
	for _, m := range manifests {
		var parsed manifest.Manifest
		if err := yaml.Unmarshal(m.bytes, &parsed); err != nil {
			log.Fatalf("Error parsing release manifest: %v", err)
		}
		final.CommitHash = parsed.CommitHash
		final.CompiledPkgs = append(final.CompiledPkgs, parsed.CompiledPkgs...)
		final.Jobs = append(final.Jobs, parsed.Jobs...)
		final.License = parsed.License
		final.Name = parsed.Name
		final.Packages = append(final.Packages, parsed.Packages...)
		final.UncommittedChanges = parsed.UncommittedChanges
		final.Version = parsed.Version
	}
	bs, err := yaml.Marshal(&final)
	if err != nil {
		log.Fatalf("Error marshaling manifest to yaml: %v", err)
	}

	if err := output.WriteHeader(&tar.Header{
		Name:    "./release.MF",
		Size:    int64(len(bs)),
		Mode:    manifests[0].header.Mode,
		Uid:     manifests[0].header.Uid,
		Gid:     manifests[0].header.Gid,
		Uname:   manifests[0].header.Uname,
		Gname:   manifests[0].header.Gname,
		ModTime: time.Now(),
	}); err != nil {
		log.Fatalf("Error writing tar header: %v", err)
	}
	if _, err := output.Write(bs); err != nil {
		log.Fatalf("Error writing manifest: %v", err)
	}
}
