package commands

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	utils2 "github.com/belphemur/CBZOptimizer/v2/internal/utils"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/pablodz/inotifywaitgo/inotifywaitgo"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	if runtime.GOOS != "linux" {
		return
	}
	command := &cobra.Command{
		Use:   "watch [folder]",
		Short: "Watch a folder for new CBZ/CBR files",
		Long:  "Watch a folder for new CBZ/CBR files.\nIt will watch a folder for new CBZ/CBR files and optimize them.",
		RunE:  WatchCommand,
		Args:  cobra.ExactArgs(1),
	}
	
	setupFormatFlag(command, &converterType, true)

	command.Flags().Uint8P("quality", "q", 85, "Quality for conversion (0-100)")
	_ = viper.BindPFlag("quality", command.Flags().Lookup("quality"))

	command.Flags().BoolP("override", "o", true, "Override the original CBZ/CBR files")
	_ = viper.BindPFlag("override", command.Flags().Lookup("override"))

	command.Flags().BoolP("split", "s", false, "Split long pages into smaller chunks")
	_ = viper.BindPFlag("split", command.Flags().Lookup("split"))

	command.Flags().DurationP("timeout", "t", 0, "Maximum time allowed for converting a single chapter (e.g., 30s, 5m, 1h). 0 means no timeout")
	_ = viper.BindPFlag("timeout", command.Flags().Lookup("timeout"))

	AddCommand(command)
}
func WatchCommand(_ *cobra.Command, args []string) error {
	path := args[0]
	if path == "" {
		return fmt.Errorf("path is required")
	}

	if !utils2.IsValidFolder(path) {
		return fmt.Errorf("the path needs to be a folder")
	}

	quality := uint8(viper.GetUint16("quality"))
	if quality <= 0 || quality > 100 {
		return fmt.Errorf("invalid quality value")
	}

	override := viper.GetBool("override")

	split := viper.GetBool("split")

	timeout := viper.GetDuration("timeout")

	converterType := constant.FindConversionFormat(viper.GetString("format"))
	chapterConverter, err := converter.Get(converterType)
	if err != nil {
		return fmt.Errorf("failed to get chapterConverter: %v", err)
	}

	err = chapterConverter.PrepareConverter()
	if err != nil {
		return fmt.Errorf("failed to prepare converter: %v", err)
	}
	log.Info().Str("path", path).Bool("override", override).Uint8("quality", quality).Str("format", converterType.String()).Bool("split", split).Msg("Watching directory")

	events := make(chan inotifywaitgo.FileEvent)
	errors := make(chan error)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		inotifywaitgo.WatchPath(&inotifywaitgo.Settings{
			Dir:        path,
			FileEvents: events,
			ErrorChan:  errors,
			Options: &inotifywaitgo.Options{
				Recursive: true,
				Events: []inotifywaitgo.EVENT{
					inotifywaitgo.MOVE,
					inotifywaitgo.CLOSE_WRITE,
				},
				Monitor: true,
			},
			Verbose: true,
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range events {
			log.Debug().Str("file", event.Filename).Interface("events", event.Events).Msg("File event")

			filename := strings.ToLower(event.Filename)
			if !strings.HasSuffix(filename, ".cbz") && !strings.HasSuffix(filename, ".cbr") {
				continue
			}

			for _, e := range event.Events {
				switch e {
				case inotifywaitgo.CLOSE_WRITE, inotifywaitgo.MOVE:
					err := utils2.Optimize(&utils2.OptimizeOptions{
						ChapterConverter: chapterConverter,
						Path:             event.Filename,
						Quality:          quality,
						Override:         override,
						Split:            split,
						Timeout:          timeout,
					})
					if err != nil {
						errors <- fmt.Errorf("error processing file %s: %w", event.Filename, err)
					}
				default:
					// ignored
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for err := range errors {
			log.Error().Err(err).Msg("Watch error")
		}
	}()

	wg.Wait()
	return nil
}
