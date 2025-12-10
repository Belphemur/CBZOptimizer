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

// setupQualityFlag sets up the quality flag for a command.
//
// Parameters:
//   - cmd: The Cobra command to add the quality flag to
//   - defaultValue: The default quality value (0-100)
//   - bindViper: If true, binds the flag to viper for configuration file support
func setupQualityFlag(cmd *cobra.Command, defaultValue uint8, bindViper bool) {
	cmd.Flags().Uint8P("quality", "q", defaultValue, "Quality for conversion (0-100)")
	if bindViper {
		_ = viper.BindPFlag("quality", cmd.Flags().Lookup("quality"))
	}
}

// setupOverrideFlag sets up the override flag for a command.
//
// Parameters:
//   - cmd: The Cobra command to add the override flag to
//   - defaultValue: The default override value
//   - bindViper: If true, binds the flag to viper for configuration file support
func setupOverrideFlag(cmd *cobra.Command, defaultValue bool, bindViper bool) {
	cmd.Flags().BoolP("override", "o", defaultValue, "Override the original CBZ/CBR files")
	if bindViper {
		_ = viper.BindPFlag("override", cmd.Flags().Lookup("override"))
	}
}

// setupSplitFlag sets up the split flag for a command.
//
// Parameters:
//   - cmd: The Cobra command to add the split flag to
//   - defaultValue: The default split value
//   - bindViper: If true, binds the flag to viper for configuration file support
func setupSplitFlag(cmd *cobra.Command, defaultValue bool, bindViper bool) {
	cmd.Flags().BoolP("split", "s", defaultValue, "Split long pages into smaller chunks")
	if bindViper {
		_ = viper.BindPFlag("split", cmd.Flags().Lookup("split"))
	}
}

// setupTimeoutFlag sets up the timeout flag for a command.
//
// Parameters:
//   - cmd: The Cobra command to add the timeout flag to
//   - bindViper: If true, binds the flag to viper for configuration file support
func setupTimeoutFlag(cmd *cobra.Command, bindViper bool) {
	cmd.Flags().DurationP("timeout", "t", 0, "Maximum time allowed for converting a single chapter (e.g., 30s, 5m, 1h). 0 means no timeout")
	if bindViper {
		_ = viper.BindPFlag("timeout", cmd.Flags().Lookup("timeout"))
	}
}

// setupCommonFlags sets up all common flags for optimize and watch commands.
//
// Parameters:
//   - cmd: The Cobra command to add the flags to
//   - converterType: Pointer to the ConversionFormat variable that will store the format flag value
//   - qualityDefault: The default quality value (0-100)
//   - overrideDefault: The default override value
//   - splitDefault: The default split value
//   - bindViper: If true, binds all flags to viper for configuration file support
func setupCommonFlags(cmd *cobra.Command, converterType *constant.ConversionFormat, qualityDefault uint8, overrideDefault bool, splitDefault bool, bindViper bool) {
	setupFormatFlag(cmd, converterType, bindViper)
	setupQualityFlag(cmd, qualityDefault, bindViper)
	setupOverrideFlag(cmd, overrideDefault, bindViper)
	setupSplitFlag(cmd, splitDefault, bindViper)
	setupTimeoutFlag(cmd, bindViper)
}
