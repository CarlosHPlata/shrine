package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

const getJSONUsage = "Print the listing as JSON instead of a table"

const getWorkloadsJSONHelp = `

With --json, the listing is printed as one JSON document, {"data": [...]},
with one entry per row carrying what describe shows: team, name, kind, image,
pullPolicy, version, pin (reference, version, pinnedAt; null when the artifact
is not pinned), runningImage (read from the container runtime; null when it
cannot be read), the full containerId and the full configHash.`

var getJSONFlag bool

// jsonQueryBackend never fails a listing: a runtime that cannot be reached
// only leaves the running images null.
func jsonQueryBackend() engine.ContainerBackend {
	backend, err := app.NewQueryContainerBackend(cfg, store)
	if err != nil {
		return nil
	}
	return backend
}

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long:  `List resources registered in the platform state.`,
}

var getTeamsCmd = &cobra.Command{
	Use:     "teams",
	Aliases: []string{"team"},
	Short:   "List all registered teams",
	Long: `Display all teams currently registered in the platform state.

With --json, the teams are printed as one JSON document, {"data": [...]}, with
name, displayName, maxApps and maxResources for each team.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if getJSONFlag {
			return handler.ListTeamsJSON(store.Teams)
		}
		return handler.ListTeams(store.Teams)
	},
}

var getConfigDir = &cobra.Command{
	Use:   "config-dir",
	Short: "Display the configuration directory",
	Long:  `Display the configuration directory.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("%s\n", paths.ConfigDir)
		return nil
	},
}

var getStateDir = &cobra.Command{
	Use:   "state-dir",
	Short: "Display the state directory",
	Long:  `Display the state directory.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("%s\n", paths.StateDir)
		return nil
	},
}

var getTeamFlag string

var getApplicationsCmd = &cobra.Command{
	Use:     "applications",
	Aliases: []string{"apps"},
	Short:   "List deployed applications",
	Long: `Display all deployed applications, optionally filtered by team.

The VERSION column shows the version each artifact was deployed with: for a
Pinned artifact, the readable version its pin was resolved from and the first
twelve characters of its exact version, such as latest@3f2a9c1b4d7e; for any
other artifact, the image reference its manifest named. The table is read from
state, so it needs no container runtime.` + getWorkloadsJSONHelp,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if getJSONFlag {
			return handler.ListApplicationsJSON(getTeamFlag, store, jsonQueryBackend())
		}
		return handler.ListApplications(getTeamFlag, store)
	},
}

var getResourcesCmd = &cobra.Command{
	Use:     "resources",
	Aliases: []string{"res"},
	Short:   "List deployed resources",
	Long: `Display all deployed resources, optionally filtered by team.

The VERSION column shows the version each artifact was deployed with: for a
Pinned artifact, the readable version its pin was resolved from and the first
twelve characters of its exact version, such as latest@3f2a9c1b4d7e; for any
other artifact, the image reference its manifest named. The table is read from
state, so it needs no container runtime.` + getWorkloadsJSONHelp,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if getJSONFlag {
			return handler.ListResourcesJSON(getTeamFlag, store, jsonQueryBackend())
		}
		return handler.ListResources(getTeamFlag, store)
	},
}

var getDeployedCmd = &cobra.Command{
	Use:   "deployed",
	Short: "List all deployed workloads",
	Long: `Display all deployed applications and resources, optionally filtered by team.

The VERSION column shows the version each artifact was deployed with: for a
Pinned artifact, the readable version its pin was resolved from and the first
twelve characters of its exact version, such as latest@3f2a9c1b4d7e; for any
other artifact, the image reference its manifest named. The table is read from
state, so it needs no container runtime.` + getWorkloadsJSONHelp,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if getJSONFlag {
			return handler.ListDeployedJSON(getTeamFlag, store, jsonQueryBackend())
		}
		return handler.ListDeployed(getTeamFlag, store)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.AddCommand(getTeamsCmd)
	getCmd.AddCommand(getConfigDir)
	getCmd.AddCommand(getStateDir)
	getCmd.PersistentFlags().StringVarP(&getTeamFlag, "team", "t", "", "Filter by team name")
	getCmd.AddCommand(getApplicationsCmd)
	getCmd.AddCommand(getResourcesCmd)
	getCmd.AddCommand(getDeployedCmd)
	for _, listing := range []*cobra.Command{getTeamsCmd, getApplicationsCmd, getResourcesCmd, getDeployedCmd} {
		listing.Flags().BoolVar(&getJSONFlag, "json", false, getJSONUsage)
	}
}
