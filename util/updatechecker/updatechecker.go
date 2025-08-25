package updatechecker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/logger"
)

// Status represents the update check result
type Status int

const (
	StatusUpToDate Status = iota
	StatusPatchUpdate
	StatusMinorUpdate
	StatusMajorUpdate
	StatusError
)

func RunUpdateChecker(Log *logger.Log, url string, major, minor, patch int) {
	Log.Info("Checking for updates")
	status, version, err := CheckForUpdate(url, major, minor, patch)
	if err != nil || status == StatusError {
		Log.Warn("Error checking for updates:", err)
		return
	}

	curVersion := fmt.Sprintf("%d.%d.%d", config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)
	if status == StatusUpToDate {
		Log.Infof("The node is up to date (v%v)", curVersion)
	} else {
		kind := "major"
		switch status {
		case StatusMinorUpdate:
			kind = "minor"
		case StatusPatchUpdate:
			kind = "patch"
		}
		Log.Infof("There's a new %s update available: You are on v%v, version v%v", kind,
			curVersion, version)
	}
}

type githubReleaseInfo struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdate checks if a newer version is available
func CheckForUpdate(url string, major, minor, patch int) (Status, string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return StatusError, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return StatusError, "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StatusError, "", err
	}

	ghr := githubReleaseInfo{}

	err = json.Unmarshal(body, &ghr)
	if err != nil {
		return StatusError, "", err
	}

	version := strings.Split(strings.TrimPrefix(ghr.TagName, "v"), "-")
	if len(version) == 0 {
		return StatusError, "", fmt.Errorf("invalid tag name %v", ghr.TagName)
	}

	remoteVersion := strings.TrimSpace(version[0])
	status, err := compareVersions(remoteVersion, major, minor, patch)
	return status, remoteVersion, err
}

// compareVersions compares the remote version with the current version
func compareVersions(remoteVersion string, major, minor, patch int) (Status, error) {
	// Parse remote version
	remoteParts := strings.Split(remoteVersion, ".")
	if len(remoteParts) != 3 {
		return StatusError, fmt.Errorf("invalid version format: %s", remoteVersion)
	}

	remoteMajor, err := strconv.Atoi(remoteParts[0])
	if err != nil {
		return StatusError, err
	}

	remoteMinor, err := strconv.Atoi(remoteParts[1])
	if err != nil {
		return StatusError, err
	}

	remotePatch, err := strconv.Atoi(remoteParts[2])
	if err != nil {
		return StatusError, err
	}

	// Compare versions
	if remoteMajor > major {
		return StatusMajorUpdate, nil
	}

	if remoteMajor == major && remoteMinor > minor {
		return StatusMinorUpdate, nil
	}

	if remoteMajor == major && remoteMinor == minor && remotePatch > patch {
		return StatusPatchUpdate, nil
	}

	return StatusUpToDate, nil
}
