package commands

import (
	"fmt"

	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thediveo/enumflag/v2"
)

// setupFormatFlag sets up the format flag for a command
// If bindViper is true, it will also bind the flag to viper
func setupFormatFlag(cmd *cobra.Command, converterType *constant.ConversionFormat, bindViper bool) {
	formatFlag := enumflag.New(converterType, "format", constant.CommandValue, enumflag.EnumCaseInsensitive)
	_ = formatFlag.RegisterCompletion(cmd, "format", constant.HelpText)
	
	cmd.Flags().VarP(
		formatFlag,
		"format", "f",
		fmt.Sprintf("Format to convert the images to: %s", constant.ListAll()))
	
	if bindViper {
		_ = viper.BindPFlag("format", cmd.Flags().Lookup("format"))
	}
}
