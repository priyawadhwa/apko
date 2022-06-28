// Copyright 2022 Chainguard, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v3"
)

// Attempt to probe an upstream VCS URL if known.
func (ic *ImageConfiguration) ProbeVCSUrl(imageConfigPath string) {
	parentDir := filepath.Dir(imageConfigPath)
	if parentDir == "" {
		return
	}

	repo, err := git.PlainOpen(parentDir)
	if err != nil {
		log.Printf("unable to determine git vcs url: %v", err)
		return
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		log.Printf("unable to determine git vcs url: %v", err)
		return
	}

	remoteConfig := remote.Config()
	remoteURL := remoteConfig.URLs[0]

	normalizedURL, err := url.Parse(remoteURL)
	if err != nil {
		// URL is most likely a git+ssh:// type URL, represented
		// in the way git itself does so.

		// Take the user@host:repo and turn it into user@host/repo.
		remoteURL = strings.Replace(remoteURL, ":", "/", 1)
		remoteURL = fmt.Sprintf("git+ssh://%s", remoteURL)

		normalizedURL, err = url.Parse(remoteURL)
		if err != nil {
			log.Printf("unable to parse %s as a git vcs url: %v", remoteURL, err)
			return
		}
	}

	ic.VCSUrl = normalizedURL.String()
	log.Printf("detected %s as VCS URL", ic.VCSUrl)
}

// Loads an image configuration given a configuration file path.
func (ic *ImageConfiguration) Load(imageConfigPath string) error {
	data, err := os.ReadFile(imageConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read image configuration file: %w", err)
	}

	if err := yaml.Unmarshal(data, ic); err != nil {
		return fmt.Errorf("failed to parse image configuration: %w", err)
	}

	if ic.VCSUrl == "" {
		ic.ProbeVCSUrl(imageConfigPath)
	}

	return nil
}

// Do preflight checks and mutations on an image configuration.
func (ic *ImageConfiguration) Validate() error {
	if ic.Entrypoint.Type == "service-bundle" {
		if err := ic.ValidateServiceBundle(); err != nil {
			return err
		}
	}

	for _, u := range ic.Accounts.Users {
		if u.UserName == "" {
			return fmt.Errorf("configured user %v has no configured user name", u)
		}

		if u.UID == 0 {
			return fmt.Errorf("configured user %v has UID 0", u)
		}
	}

	for _, g := range ic.Accounts.Groups {
		if g.GroupName == "" {
			return fmt.Errorf("configured group %v has no configured group name", g)
		}

		if g.GID == 0 {
			return fmt.Errorf("configured group %v has GID 0", g)
		}
	}

	if ic.OSRelease.ID == "" {
		ic.OSRelease.ID = "alpine"
	}

	if ic.OSRelease.Name == "" {
		ic.OSRelease.Name = "apko-generated image"
		ic.OSRelease.PrettyName = "apko-generated image"
	}

	if ic.OSRelease.VersionID == "" {
		ic.OSRelease.VersionID = "3.16"
	}

	if ic.OSRelease.HomeURL == "" {
		ic.OSRelease.HomeURL = "https://github.com/chainguard-dev/apko"
	}

	return nil
}

// Do preflight checks and mutations on an image configured to manage
// a service bundle.
func (ic *ImageConfiguration) ValidateServiceBundle() error {
	ic.Entrypoint.Command = "/bin/s6-svscan /sv"

	// It's harmless to have a duplicate entry in /etc/apk/world,
	// apk will fix it up when the fixate op happens.
	ic.Contents.Packages = append(ic.Contents.Packages, "s6")

	return nil
}

func (ic *ImageConfiguration) Summarize(logger *log.Logger) {
	logger.Printf("image configuration:")
	logger.Printf("  contents:")
	logger.Printf("    repositories: %v", ic.Contents.Repositories)
	logger.Printf("    keyring:      %v", ic.Contents.Keyring)
	logger.Printf("    packages:     %v", ic.Contents.Packages)
	if ic.Entrypoint.Type != "" || ic.Entrypoint.Command != "" || len(ic.Entrypoint.Services) != 0 {
		logger.Printf("  entrypoint:")
		logger.Printf("    type:    %s", ic.Entrypoint.Type)
		logger.Printf("    command:     %s", ic.Entrypoint.Command)
		logger.Printf("    service: %v", ic.Entrypoint.Services)
		logger.Printf("    shell fragment: %v", ic.Entrypoint.ShellFragment)
	}
	if ic.Cmd != "" {
		logger.Printf("  cmd: %s", ic.Cmd)
	}
	if ic.Accounts.RunAs != "" || len(ic.Accounts.Users) != 0 || len(ic.Accounts.Groups) != 0 {
		logger.Printf("  accounts:")
		logger.Printf("    runas:  %s", ic.Accounts.RunAs)
		logger.Printf("    users:")
		for _, u := range ic.Accounts.Users {
			logger.Printf("      - uid=%d(%s) gid=%d", u.UID, u.UserName, u.GID)
		}
		logger.Printf("    groups:")
		for _, g := range ic.Accounts.Groups {
			logger.Printf("      - gid=%d(%s) members=%v", g.GID, g.GroupName, g.Members)
		}
	}
}
