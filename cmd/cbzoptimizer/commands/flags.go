package commands

import (
	"fmt"

	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thediveo/enumflag/v2"
)

// setupFormatFlag sets up the format flag for a command.
//
// Parameters:
//   - cmd: The Cobra command to add the format flag to
//   - converterType: Pointer to the ConversionFormat variable that will store the flag value
//   - bindViper: If true, binds the flag to viper for configuration file support.
//     Set to true for commands that use viper for configuration (e.g., watch command),
//     and false for commands that don't (e.g., optimize command).
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
