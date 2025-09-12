package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fleetd.sh/internal/provision"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage fleetp image cache",
	Long:  `Manage the local cache of OS images and decompressed images used by fleetp.`,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the image cache",
	Long:  `Clear the cached OS images and decompressed images to free up disk space.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		clearDecompressed, _ := cmd.Flags().GetBool("decompressed-only")
		clearAll, _ := cmd.Flags().GetBool("all")

		imageManager := provision.NewImageManager("")

		if clearAll {
			// Clear everything
			home, _ := os.UserHomeDir()
			cacheDir := filepath.Join(home, ".fleetd", "images")
			if err := os.RemoveAll(cacheDir); err != nil {
				return fmt.Errorf("failed to clear cache: %w", err)
			}
			fmt.Println("Cleared all cached images")
		} else if clearDecompressed {
			// Clear only decompressed images
			if err := imageManager.ClearDecompressedCache(); err != nil {
				return fmt.Errorf("failed to clear decompressed cache: %w", err)
			}
			fmt.Println("Cleared decompressed image cache")
		} else {
			fmt.Println("Please specify --all or --decompressed-only")
			return fmt.Errorf("no cache type specified")
		}

		return nil
	},
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached images",
	Long:  `List all cached OS images and decompressed images with their sizes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		cacheDir := filepath.Join(home, ".fleetd", "images")
		decompressedDir := filepath.Join(cacheDir, "decompressed")

		fmt.Println("Cached images:")
		fmt.Println()

		// List compressed images
		fmt.Println("Compressed images:")
		compressedSize := int64(0)
		err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && !strings.Contains(path, "/decompressed/") {
				fmt.Printf("  %s (%.1f MB)\n", filepath.Base(path), float64(info.Size())/1024/1024)
				compressedSize += info.Size()
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to list compressed cache: %w", err)
		}

		fmt.Println()
		fmt.Println("Decompressed images:")
		decompressedSize := int64(0)
		err = filepath.Walk(decompressedDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				fmt.Printf("  %s (%.1f MB)\n", filepath.Base(path), float64(info.Size())/1024/1024)
				decompressedSize += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to list decompressed cache: %w", err)
		}

		fmt.Println()
		fmt.Printf("Total compressed cache size: %.1f MB\n", float64(compressedSize)/1024/1024)
		fmt.Printf("Total decompressed cache size: %.1f MB\n", float64(decompressedSize)/1024/1024)
		fmt.Printf("Total cache size: %.1f MB\n", float64(compressedSize+decompressedSize)/1024/1024)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheListCmd)

	cacheClearCmd.Flags().Bool("decompressed-only", false, "Clear only decompressed images")
	cacheClearCmd.Flags().Bool("all", false, "Clear all cached images (compressed and decompressed)")
}
