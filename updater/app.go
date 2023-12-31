// Package updater provides functionality to automatically update a Go application
// by checking for the latest release on GitHub and, if available, downloading and
// applying the update. It is designed to work with applications that are distributed
// with GitHub releases.
//
// The updater checks the latest release by calling the GitHub Releases API and
// compares the tag name of the latest release with the current version of the
// application. If the tag name indicates a newer version, the updater downloads
// the release asset that matches the running application's operating system and
// architecture, replaces the current executable, and restarts the application.
//
// Usage:
//
// To use the updater, you should include it in your application's main package:
//
//	import "github.com/H0llyW00dzZ/ChatGPT-Next-Web-Session-Exporter/updater"
//
//	func main() {
//	    if err := updater.UpdateApplication(); err != nil {
//	        // Handle error
//	    }
//	    // Continue with application logic
//	}
//
// The updater assumes that the GitHub repository's release assets follow a
// naming convention that includes the OS and architecture. It also assumes that
// the binary to be updated is named "myapp" and is located in the current working
// directory of the running application.
//
// Note that the updater package defines a constant `currentVersion` that must
// be updated to match the application's current version string before building
// a new release. This version string is used to compare against the tag name of
// the latest release on GitHub.
//
// The updater package is designed with simplicity in mind and does not handle
// complex update scenarios such as database migrations, configuration changes,
// or rollback of failed updates. It is recommended to test the update process
// thoroughly in a controlled environment before deploying it in a production setting.
//
// Security Considerations:
//
// The updater performs a direct binary replacement and restarts the application.
// Users should ensure that the GitHub repository and release assets are secure
// and that the release process includes steps to verify the integrity and
// authenticity of the binaries, such as signing the releases.
//
// # Additional Note: This Package Currently under development.
//
// Copyright (c) 2023 H0llyW00dzZ
package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/H0llyW00dzZ/ChatGPT-Next-Web-Session-Exporter/filesystem"
	"github.com/H0llyW00dzZ/ChatGPT-Next-Web-Session-Exporter/interactivity"
)

const (
	currentVersion = "1.3.3.7"
	githubRepo     = "H0llyW00dzZ/ChatGPT-Next-Web-Session-Exporter"
)

// releaseInfo defines the structure for storing information about a GitHub release.
// It captures the tag name of the release and a slice of assets that are part of the release.
type releaseInfo struct {
	TagName string `json:"tag_name"` // The name of the tag for the release.
	Body    string `json:"body"`     // The release notes or description.
	Assets  []struct {
		Name               string `json:"name"`                 // The name of the asset.
		BrowserDownloadURL string `json:"browser_download_url"` // The URL for downloading the asset.
	} `json:"assets"` // A list of assets available for the release.
}

// printReleaseNotes takes a string containing the body of a GitHub release,
// which is typically formatted using Markdown, and prints it to the console
// with some basic formatting applied for improved readability.
//
// The function performs the following transformations:
//   - Converts Markdown headings (denoted by "## ") into all-uppercase text
//     preceded by a newline, to visually separate sections when printed.
//   - Normalizes newline characters across different operating systems to ensure
//     consistent line breaks.
//   - Splits the body into individual lines and inspects each line to detect URLs.
//
// Lines that contain URLs are prefixed with "Link:" to highlight them as clickable
// links, even though they are not actually clickable in the terminal output.
// This is a simple heuristic and may not recognize all URLs, especially if they
// do not start with "http" or are part of Markdown link syntax.
//
// This function does not fully render Markdown as seen in web browsers. It is
// intended to provide a basic, text-only representation that is suitable for
// terminal output. For more complex Markdown rendering, a dedicated Markdown
// parser would be more appropriate.
//
// Parameters:
// - body: The Markdown-formatted release notes as a string.
//
// Note: The current implementation is a simple approach and may not be suitable
// for all Markdown features. It is recommended to use a proper Markdown parser
// for a more accurate and complete transformation.
func printReleaseNotes(body string) {
	// Replace Markdown headings with all-uppercase plus newlines
	body = strings.ReplaceAll(body, "## ", "\n")
	body = strings.ReplaceAll(body, "\r\n", "\n") // Normalize newlines for cross-platform compatibility

	// Print each line, checking if it's a link to format it differently
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if strings.Contains(line, "https://") {
			// This check assumes that any URLs will use HTTPS.
			// In a real application, consider using a more robust method
			// to detect both HTTP and HTTPS links.
			fmt.Printf("Link: %s\n", line)
		} else {
			fmt.Println(line)
		}
	}
}

// getLatestRelease fetches the latest release information from the GitHub repository.
// It constructs a request to the GitHub API to retrieve the latest release and parses
// the response into a releaseInfo struct.
//
// Returns a pointer to a releaseInfo struct and nil error on success.
// On failure, it returns nil and an error indicating what went wrong.
func getLatestRelease() (*releaseInfo, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API response status: %s", resp.Status)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// UpdateApplication checks the GitHub repository for a newer release of the application.
// If a newer release is found, it downloads the corresponding binary for the current
// platform and architecture, replaces the current executable with the downloaded binary,
// and restarts the application.
//
// Returns nil if the application is up to date or the update is successfully applied.
// If an error occurs during the update process, it returns a non-nil error.
func UpdateApplication(rfs filesystem.FileSystem) error {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)
	release, err := getLatestRelease()
	if err != nil {
		return fmt.Errorf("error fetching latest release: %w", err)
	}

	if release.TagName == currentVersion {
		fmt.Println("No update available.")
		return nil
	}

	// Print release notes
	fmt.Printf("Release notes for version %s:\n", release.TagName)
	printReleaseNotes(release.Body)

	// Pass only the release to downloadAndUpdate
	tempFileName, err := downloadAndUpdate(release)
	if err != nil {
		return err
	}

	// Pass the context, reader, and filesystem to applyUpdate
	if err := applyUpdate(ctx, reader, rfs, tempFileName); err != nil {
		return err
	}

	restartApplication()
	return nil
}

// downloadAndUpdate handles the downloading and updating of the application.
// It returns the name of the downloaded file or an error.
func downloadAndUpdate(release *releaseInfo) (string, error) {
	fmt.Printf("Update available: %s\n", release.TagName)
	fmt.Println("Downloading update...")

	assetURL, err := findMatchingAsset(release)
	if err != nil {
		return "", err
	}

	tempFileName, err := downloadAsset(assetURL)
	if err != nil {
		return "", err
	}

	fmt.Println("Update downloaded.")
	return tempFileName, nil
}

// findMatchingAsset finds and returns the URL of the asset that matches the current platform.
func findMatchingAsset(release *releaseInfo) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == fmt.Sprintf("ChatGPT-Next-Web-Session-Exporter-%s-%s", runtime.GOOS, runtime.GOARCH) {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no binary for the current platform")
}

// downloadAsset downloads the asset from the given URL and writes it to a temporary file.
// It returns the name of the temporary file or an error.
func downloadAsset(assetURL string) (string, error) {
	resp, err := http.Get(assetURL)
	if err != nil {
		return "", fmt.Errorf("error downloading update: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.CreateTemp("", "ChatGPT-Next-Web-Session-Exporter-update-*")
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return out.Name(), nil
}

// applyUpdate applies the update by replacing the current binary with the new one.
// It takes the name of the temporary file containing the new binary as an argument.
func applyUpdate(ctx context.Context, reader *bufio.Reader, rfs filesystem.FileSystem, tempFileName string) error {
	// Confirm whether to overwrite the existing binary
	shouldOverwrite, err := interactivity.ConfirmOverwrite(rfs, ctx, reader, "ChatGPT-Next-Web-Session-Exporter")
	if err != nil {
		return fmt.Errorf("error during overwrite confirmation: %w", err)
	}
	if !shouldOverwrite {
		fmt.Println("Update cancelled by the user.")
		return nil
	}

	// Replace the current binary with the new one
	if err := os.Rename(tempFileName, "ChatGPT-Next-Web-Session-Exporter"); err != nil {
		return fmt.Errorf("error replacing binary: %w", err)
	}
	return nil
}

// restartApplication restarts the application.
func restartApplication() {
	fmt.Println("Update applied. Restarting application...")
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error restarting application: %v", err)
		return
	}

	// Exit the current process
	os.Exit(0)
}
