package main

import (
	"encoding/json"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var rootCmd = &cobra.Command{
	Use:   strings.ToLower(AppName),
	Short: AppName + " - Manga Downloader",
	Long:  `A fast and flexible manga downloader`,
	Run: func(cmd *cobra.Command, args []string) {
		config, _ := cmd.Flags().GetString("config")
		initConfig(config)

		var program *tea.Program

		if UserConfig.Fullscreen {
			program = tea.NewProgram(NewBubble(searchState), tea.WithAltScreen())
		} else {
			program = tea.NewProgram(NewBubble(searchState))
		}

		if err := program.Start(); err != nil {
			log.Fatal(err)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Long:  fmt.Sprintf("Shows %s versions and build date", AppName),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", AppName, version)
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove cached and temp files",
	Long:  "Removes cached files produced by scraper and temp files from downloader",
	Run: func(cmd *cobra.Command, args []string) {
		var (
			// counter of removed files
			counter int
			// bytes removed
			bytes int64
		)

		leaveCache, _ := cmd.Flags().GetBool("preserve-cache")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Cleanup temp files
		tempDir := os.TempDir()
		tempFiles, err := Afero.ReadDir(tempDir)
		if err == nil {
			lowerAppName := strings.ToLower(AppName)
			for _, tempFile := range tempFiles {
				name := tempFile.Name()
				if strings.HasPrefix(name, AppName) || strings.HasPrefix(name, lowerAppName) {

					p := filepath.Join(tempDir, name)

					if tempFile.IsDir() {
						b, err := DirSize(p)
						if err == nil {
							bytes += b
						}
					}

					// Print out removed file
					if verbose {
						fmt.Println(p)
					}
					err = Afero.RemoveAll(p)
					if err == nil {
						bytes += tempFile.Size()
						counter++
					}
				}
			}
		}

		if !leaveCache {
			// Cleanup cache files
			cacheDir, err := os.UserCacheDir()
			if err == nil {
				scraperCacheDir := filepath.Join(cacheDir, CachePrefix)
				if exists, err := Afero.Exists(scraperCacheDir); err == nil && exists {
					files, err := Afero.ReadDir(scraperCacheDir)
					if err == nil {
						for _, f := range files {
							counter++
							bytes += f.Size()
						}
					}

					// Print out removed cache folder
					if verbose {
						fmt.Printf("%s [%d %s]\n", scraperCacheDir, len(files), Plural("file", len(files)))
					}
					_ = Afero.RemoveAll(scraperCacheDir)
				}
			}
		}

		fmt.Printf("%d files removed\nCleaned up %.2fMB\n", counter, BytesToMegabytes(bytes))
	},
}

var inlineCmd = &cobra.Command{
	Use:   "inline",
	Short: "Search & Download manga in inline mode",
	Long: `Search & Download manga in inline mode
Useful for scripting`,
	Run: func(cmd *cobra.Command, args []string) {
		config, _ := cmd.Flags().GetString("config")
		initConfig(config)

		var (
			manga []*URL
			wg    sync.WaitGroup
		)

		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			log.Fatal("Query expected")
		}

		wg.Add(len(UserConfig.Scrapers))

		for _, scraper := range UserConfig.Scrapers {
			go func(s *Scraper) {
				defer wg.Done()

				m, err := s.SearchManga(query)

				if err == nil {
					manga = append(manga, m...)
				}
			}(scraper)
		}

		wg.Wait()

		showUrls, _ := cmd.Flags().GetBool("urls")
		asJson, _ := cmd.Flags().GetBool("json")
		if mangaIdx, _ := cmd.Flags().GetInt("manga"); mangaIdx != -1 {
			if mangaIdx > len(manga) || mangaIdx <= 0 {
				log.Fatal("Index out of range")
			}

			selectedManga := manga[mangaIdx-1]

			chapters, err := selectedManga.Scraper.GetChapters(selectedManga)
			if err != nil {
				log.Fatal("Error while getting chapters")
			}

			if chapterIdx, _ := cmd.Flags().GetInt("chapter"); chapterIdx != -1 {

				selectedChapter, ok := Find(chapters, func(c *URL) bool {
					return c.Index == chapterIdx
				})

				if !ok {
					log.Fatal("Index out of range")
				}

				asTemp, _ := cmd.Flags().GetBool("temp")
				chapterPath, err := DownloadChapter(selectedChapter, nil, asTemp)
				if err != nil {
					log.Fatal("Error while downloading chapter")
				}

				fmt.Println(chapterPath)
				return
			}

			if asJson {
				data, err := json.Marshal(chapters)
				if err != nil {
					log.Fatal("Could not get data as json")
				}

				fmt.Println(string(data))
			} else {
				for _, c := range chapters {
					if showUrls {
						fmt.Printf("[%d] %s %s\n", c.Index, c.Info, c.Address)
					} else {
						fmt.Printf("[%d] %s\n", c.Index, c.Info)
					}
				}
			}

		} else {
			if asJson {
				data, err := json.Marshal(manga)
				if err != nil {
					log.Fatal("Could not get data as json")
				}

				fmt.Println(string(data))
			} else {
				for i, m := range manga {
					if showUrls {
						fmt.Printf("[%d] %s %s\n", i+1, m.Info, m.Address)
					} else {
						fmt.Printf("[%d] %s\n", i+1, m.Info)
					}
				}
			}
		}
	},
}

func initConfig(config string) {
	exists, err := Afero.Exists(config)

	if err != nil {
		log.Fatal(errors.New("access to config file denied"))
	}

	if config != "" {
		config = path.Clean(config)
		if !exists {
			log.Fatal(errors.New(fmt.Sprintf("config at path %s doesn't exist", config)))
		}

		UserConfig = GetConfig(config)
	} else {
		UserConfig = GetConfig("") // get config from default config path
	}
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Config manipulation",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configWhereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show config location",
	Long:  "Show path where config is located if it exists.\nOtherwise show path where it is expected to be",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Can't get config location, permission denied, probably")
		}

		exists, err := Afero.Exists(configPath)

		if err != nil {
			log.Fatalf("Can't understand if config exists or not. It is expected at\n%s\n", configPath)
		}

		if exists {
			fmt.Printf("Config exists at\n%s\n", configPath)
		} else {
			fmt.Printf("Config doesn't exist, but it is expected to be at\n%s\n", configPath)
		}
	},
}

var configPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview current config",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Permission to config file was denied")
		}

		exists, err := Afero.Exists(configPath)

		if err != nil {
			log.Fatalf("Permission to config file was denied")
		}

		if !exists {
			log.Fatal("Config doesn't exist")
		}

		contents, err := Afero.ReadFile(configPath)
		if err != nil {
			log.Fatal("Permission to config file was denied")
		}

		fmt.Println(string(contents))
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit config in the default editor",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Permission to config file was denied")
		}

		err = open.Start(configPath)
		if err != nil {
			log.Fatal("Can't open editor")
		}
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Init default config",
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")

		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Can't get config location, permission denied, probably")
		}

		exists, err := Afero.Exists(configPath)

		var createConfig = func() {
			if err := Afero.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
				log.Fatal("Error while creating file")
			} else if file, err := Afero.Create(configPath); err != nil {
				log.Fatal("Error while creating file")
			} else if _, err = file.Write(DefaultConfigBytes); err != nil {
				log.Fatal("Error while writing to file")
			} else {
				fmt.Printf("Config created at\n%s\n", configPath)
			}
		}

		if force {
			createConfig()
			return
		}

		if err != nil {
			log.Fatalf("Can't understand if config exists or not, but it is expected at\n%s\n", configPath)
		}

		if exists {
			log.Fatal("Config file already exists. Use --force to overwrite it")
		} else {
			createConfig()
		}
	},
}

var configTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test user config for any errors",
	Run: func(cmd *cobra.Command, args []string) {
		initConfig("")

		if err := ValidateConfig(UserConfig); err != nil {
			log.Fatal(err)
		} else {
			fmt.Println("Everything is OK")
		}
	},
}

func CmdExecute() {
	rootCmd.AddCommand(versionCmd)

	cleanupCmd.Flags().BoolP("preserve-cache", "c", false, "do not remove cache")
	cleanupCmd.Flags().BoolP("verbose", "v", false, "print out removed files")
	rootCmd.AddCommand(cleanupCmd)

	configCmd.AddCommand(configWhereCmd)
	configCmd.AddCommand(configPreviewCmd)
	configCmd.AddCommand(configEditCmd)
	configInitCmd.Flags().BoolP("force", "f", false, "overwrite existing config")
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configTestCmd)
	rootCmd.AddCommand(configCmd)

	inlineCmd.Flags().Int("manga", -1, "choose manga by index")
	inlineCmd.Flags().Int("chapter", -1, "choose and download chapter by index")
	inlineCmd.Flags().StringP("query", "q", "", "manga to search")
	inlineCmd.Flags().BoolP("json", "j", false, "print as json")
	inlineCmd.Flags().BoolP("urls", "u", false, "show urls")
	inlineCmd.Flags().BoolP("temp", "t", false, "download as temp")
	inlineCmd.Flags().StringP("config", "c", "", "use config from path")
	inlineCmd.Flags().SortFlags = false
	rootCmd.AddCommand(inlineCmd)

	rootCmd.Flags().StringP("config", "c", "", "use config from path")

	_ = rootCmd.Execute()
}
