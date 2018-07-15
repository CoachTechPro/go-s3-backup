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

package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/robfig/cron"
	"github.com/urfave/cli"
	log "gopkg.in/clog.v1"
	"megpoid.xyz/go/go-s3-backup/services"
	"megpoid.xyz/go/go-s3-backup/stores"
)

type task func(c *cli.Context) error

func getService(c *cli.Context, service string) services.Service {
	var config services.Service
	switch service {
	case "gogs":
		config = newGogsConfig(c)
	case "mysql":
		config = newMysqlConfig(c)
	case "postgres":
		config = newPostgresConfig(c)
	case "tarball":
		config = newTarballConfig(c)
	default:
		log.Fatal(0, "Unsupported service: %s", service)
	}

	return config
}

func getStore(c *cli.Context, store string) stores.Storer {
	var config stores.Storer
	switch store {
	case "s3":
		config = newS3Config(c)
	case "filesystem":
		config = newFilesystemConfig(c)
	default:
		log.Fatal(0, "Unsupported store: %s", store)
	}

	return config
}

func runTask(c *cli.Context, command string, serviceName string, storeName string) error {
	service := getService(c, serviceName)
	store := getStore(c, storeName)

	switch command {
	case "backup":
		return runScheduler(c, func(c *cli.Context) error {
			return backupTask(c, service, store)
		})
	case "restore":
		return runScheduler(c, func(c *cli.Context) error {
			return restoreTask(c, service, store)
		})
	default:
		log.Fatal(0, "Unsupported command: %s", command)
	}
	return nil
}

func backupTask(c *cli.Context, service services.Service, store stores.Storer) error {
	filepath, err := service.Backup()
	if err != nil {
		return fmt.Errorf("service backup failed: %v", err)
	}

	log.Trace("Backup saved to %s", filepath)

	filename := path.Base(filepath)

	if err = store.Store(filepath, filename); err != nil {
		return fmt.Errorf("couldn't upload file to store: %v", err)
	}

	err = store.RemoveOlderBackups(c.GlobalInt("max-backups"))
	if err != nil {
		return fmt.Errorf("couldn't remove old backups from store: %v", err)
	}

	return nil
}

func restoreTask(c *cli.Context, service services.Service, store stores.Storer) error {
	var err error
	var filename string

	if key := c.GlobalString("restore-file"); key != "" {
		// restore directly from this file
		filename = key
	} else {
		// find the latest file in the store
		filename, err = store.FindLatestBackup()
		if err != nil {
			return fmt.Errorf("cannot find the latest backup: %v", err)
		}
	}

	filepath, err := store.Retrieve(filename)
	if err != nil {
		return fmt.Errorf("cannot download file %s: %v", filename, err)
	}

	defer store.Close()

	log.Trace("Backup retrieved to %s", filepath)

	if err = service.Restore(filepath); err != nil {
		return fmt.Errorf("service restore failed: %v", err)
	}

	return nil
}

func runScheduler(c *cli.Context, task task) error {
	cr := cron.New()
	schedule := c.GlobalString("schedule")

	if schedule == "" || schedule == "none" {
		log.Trace("Running task directly")
		return task(c)
	}

	log.Trace("Starting scheduled backup task")
	timeoutchan := make(chan bool, 1)

	cr.AddFunc(schedule, func() {
		delay := c.GlobalInt("random-delay")
		if delay <= 0 {
			log.Warn("Schedule random delay was set to a number <= 0, using 1 as default")
			delay = 1
		}

		seconds := rand.Intn(delay)

		// run immediately is no delay is configured
		if seconds == 0 {
			if err := task(c); err != nil {
				log.Error(0, "Failed to run scheduled task: %v", err)
			}
			return
		}

		log.Trace("Waiting for %d seconds before starting scheduled job", seconds)

		select {
		case <-timeoutchan:
			log.Trace("Random timeout cancelled")
			break
		case <-time.After(time.Duration(seconds) * time.Second):
			log.Trace("Running scheduled task")

			if err := task(c); err != nil {
				log.Error(0, "Failed to run scheduled task: %v", err)
			}
			break
		}
	})
	cr.Start()

	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	<-signalChan
	timeoutchan <- true
	close(timeoutchan)

	log.Trace("Stopping scheduled task")
	cr.Stop()

	return nil
}

func fileOrString(c *cli.Context, name string) string {
	if filepath := c.String(name + "-file"); filepath != "" {
		f, err := os.Open(filepath)
		if err != nil {
			log.Error(0, "Cannot open password file: %v", err)
			return ""
		}

		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			return scanner.Text()
		}

		log.Warn("Using empty password file")
		return ""
	}

	return c.String(name)
}
