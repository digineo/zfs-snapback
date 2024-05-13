// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
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

package cmd

import (
	"fmt"
	"os/exec"

	"github.com/piotaixr/zfs-snapback/zfs"
	"github.com/spf13/cobra"
)

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronizes ZFS snapshots",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args:    cobra.ExactArgs(2),
	Example: "zfs-snapback sync backup@remote.host:zpool/var zpool/backup/remote.host",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		var err error

		// source
		source, err = zfs.GetFilesystem(flags, args[0])
		if err != nil {
			return fmt.Errorf("invalid source '%s': %w", args[0], err)
		}

		// source
		destination, err = zfs.GetFilesystem(flags, args[1])
		if err != nil {
			return fmt.Errorf("invalid destination '%s': %w", args[1], err)
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		checkError(zfs.DoSync(source, destination, flags))
	},
}

var (
	flags       zfs.Flags
	source      *zfs.Fs
	destination *zfs.Fs
)

func init() {
	set := syncCmd.Flags()
	set.BoolVarP(&flags.Recursive, "recursive", "r", false, "Synchronize filesystems revursively")
	set.BoolVarP(&flags.Progress, "progress", "p", false, "Show progress")
	set.BoolVarP(&flags.Force, "force", "f", false, "Force a rollback of the file system to the most recent snapshot before performing the receive operation.")
	set.BoolVarP(&flags.Raw, "raw", "w", false, "Send encrypted streams as raw.")
	set.StringVarP(&flags.Compression, "compression", "c", "", "Set the compression option for SSH (yes/no)")

	RootCmd.AddCommand(syncCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// syncCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// syncCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func checkError(err error) {
	switch err := err.(type) {
	case *exec.ExitError:
		panic(string(err.Stderr))
	case error:
		panic(err)
	}
}
