package cmd

import (
	"fmt"
	"main/common"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Penny Vault",
	Long:  `Print the version number of Penny Vault`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(common.BuildVersionString())
	},
}
