/*
Copyright 2019 The Fission Authors.

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

package timetrigger

import (
	"github.com/spf13/cobra"

	wrapper "github.com/fission/fission/pkg/fission-cli/cliwrapper/driver/cobra"
	"github.com/fission/fission/pkg/fission-cli/flag"
)

func Commands() *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a time trigger",
		RunE:  wrapper.Wrapper(Create),
	}
	wrapper.SetFlags(createCmd, flag.FlagSet{
		Optional: []flag.Flag{flag.TtName, flag.TtFnName,
			flag.TtCron, flag.NamespaceFunction,
			flag.TtMethod, flag.FnSubPath,

			flag.SpecSave, flag.SpecDry,
		},
	})

	updateCmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{},
		Short:   "Update a time trigger",
		RunE:    wrapper.Wrapper(Update),
	}
	wrapper.SetFlags(updateCmd, flag.FlagSet{
		Required: []flag.Flag{flag.TtName},
		Optional: []flag.Flag{flag.TtFnName, flag.TtCron, flag.NamespaceTrigger,

			flag.TtMethod, flag.FnSubPath,
		},
	})

	deleteCmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{},
		Short:   "Delete a time trigger",
		RunE:    wrapper.Wrapper(Delete),
	}
	wrapper.SetFlags(deleteCmd, flag.FlagSet{
		Required: []flag.Flag{flag.TtName},
		Optional: []flag.Flag{flag.NamespaceTrigger, flag.IgnoreNotFound},
	})

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{},
		Short:   "List time triggers",
		Long:    "List all time triggers in a namespace if specified, else, list time triggers across all namespaces",
		RunE:    wrapper.Wrapper(List),
	}
	wrapper.SetFlags(listCmd, flag.FlagSet{
		Optional: []flag.Flag{flag.NamespaceTrigger, flag.AllNamespaces},
	})

	showCmd := &cobra.Command{
		Use:     "showschedule",
		Aliases: []string{"show"},
		Short:   "Show schedule for cron spec",
		RunE:    wrapper.Wrapper(Show),
	}
	wrapper.SetFlags(showCmd, flag.FlagSet{
		Optional: []flag.Flag{flag.TtCron, flag.TtRound},
	})

	command := &cobra.Command{
		Use:     "timetrigger",
		Aliases: []string{"tt", "timer"},
		Short:   "Create, update and manage time triggers",
	}

	command.AddCommand(createCmd, updateCmd, deleteCmd, listCmd, showCmd)

	return command
}
