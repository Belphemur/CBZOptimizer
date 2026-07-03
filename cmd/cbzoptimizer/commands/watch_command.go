package commands

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	utils2 "github.com/belphemur/CBZOptimizer/v2/internal/utils"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
	"github.com/fsnotify/fsnotify"
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

	// Setup common flags (format, quality, override, split, timeout) with viper binding
	setupCommonFlags(command, &converterType, 85, true, false, true)

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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	defer watcher.Close()

	if err := addRecursiveWatch(watcher, path); err != nil {
		return fmt.Errorf("failed to watch path %s: %w", path, err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			log.Debug().Str("file", event.Name).Str("event", event.Op.String()).Msg("File event")

			if event.Has(fsnotify.Create) {
				fileInfo, err := os.Stat(event.Name)
				if err == nil && fileInfo.IsDir() {
					if err := addRecursiveWatch(watcher, event.Name); err != nil {
						log.Error().Err(err).Str("path", event.Name).Msg("Failed to watch created directory")
					}
				}
			}

			if !shouldProcessWatchEvent(event) {
				continue
			}

			if !isComicArchive(event.Name) {
				continue
			}

			if err := utils2.Optimize(&utils2.OptimizeOptions{
				ChapterConverter: chapterConverter,
				Path:             event.Name,
				Quality:          quality,
				Override:         override,
				Split:            split,
				Timeout:          timeout,
			}); err != nil {
				log.Error().Err(err).Str("file", event.Name).Msg("Error processing file")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Error().Err(err).Msg("Watch error")
		}
	}
}

func addRecursiveWatch(watcher *fsnotify.Watcher, rootPath string) error {
	return filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("failed to watch directory %s: %w", path, err)
		}
		return nil
	})
}

func shouldProcessWatchEvent(event fsnotify.Event) bool {
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Rename)
}

func isComicArchive(path string) bool {
	filename := strings.ToLower(path)
	return strings.HasSuffix(filename, ".cbz") || strings.HasSuffix(filename, ".cbr")
}
