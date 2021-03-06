/*
Copyright 2018 codestation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package services

import (
	"fmt"
	"path"

	"github.com/mholt/archiver/v3"
)

// TarballConfig has the config options for the TarballConfig service
type TarballConfig struct {
	Name     string
	Path     string
	Compress bool
	SaveDir  string
}

// Backup creates a tarball of the specified directory
func (f *TarballConfig) Backup() (string, error) {
	var name string
	if f.Name != "" {
		name = f.Name + "-backup"
	} else {
		name = path.Base(f.Path) + "-backup"
	}

	filepath := generateFilename(f.SaveDir, name) + ".tar"

	if f.Compress {
		filepath += ".gz"
	}

	err := archiver.Archive([]string{f.Path}, filepath)
	if err != nil {
		return "", fmt.Errorf("cannot create tarball on %s, %v", filepath, err)
	}

	return filepath, nil
}

// Restore extracts a tarball to the specified directory
func (f *TarballConfig) Restore(filepath string) error {
	err := removeDirectoryContents(f.Path)
	if err != nil {
		return fmt.Errorf("failed to empty directory contents before restoring: %v", err)
	}

	err = archiver.Unarchive(filepath, path.Dir(f.Path))
	if err != nil {
		return fmt.Errorf("cannot unpack backup: %v", err)
	}

	return nil
}
