package updater

import (
	"fmt"
	"regexp"
	"sentinel/logger"
	"strconv"
	"strings"
)

// SemVer holds a parsed semantic version
type SemVer struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// Policy defines what updates are allowed
type Policy string

const (
	PolicyAll   Policy = "all"
	PolicyMajor Policy = "major"
	PolicyMinor Policy = "minor"
	PolicyPatch Policy = "patch"
	PolicyNone  Policy = "none"
)

// ParseSemVer parses a version string into SemVer
func ParseSemVer(version string) (*SemVer, error) {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")

	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid version: %s", version)
	}

	sv := &SemVer{Raw: version}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", parts[0])
	}
	sv.Major = major

	if len(parts) > 1 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid minor version: %s", parts[1])
		}
		sv.Minor = minor
	}

	if len(parts) > 2 {
		patchStr := strings.Split(parts[2], "-")[0]
		patch, err := strconv.Atoi(patchStr)
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", parts[2])
		}
		sv.Patch = patch
	}

	return sv, nil
}

// IsNewer checks if new version is newer than current
func IsNewer(current *SemVer, new *SemVer) bool {
	if new.Major != current.Major {
		return new.Major > current.Major
	}
	if new.Minor != current.Minor {
		return new.Minor > current.Minor
	}
	return new.Patch > current.Patch
}

// IsAllowed checks if an update is allowed by policy
func IsAllowed(current *SemVer, new *SemVer, policy Policy) bool {
	switch policy {
	case PolicyNone:
		return false
	case PolicyAll, PolicyMajor:
		return IsNewer(current, new)
	case PolicyMinor:
		if new.Major > current.Major {
			logger.Log.Warnf("Policy minor: blocking major bump %s -> %s", current.Raw, new.Raw)
			return false
		}
		return IsNewer(current, new)
	case PolicyPatch:
		if new.Major > current.Major {
			logger.Log.Warnf("Policy patch: blocking major bump %s -> %s", current.Raw, new.Raw)
			return false
		}
		if new.Minor > current.Minor {
			logger.Log.Warnf("Policy patch: blocking minor bump %s -> %s", current.Raw, new.Raw)
			return false
		}
		return IsNewer(current, new)
	default:
		logger.Log.Warnf("Unknown policy: %s - allowing all", policy)
		return IsNewer(current, new)
	}
}

// CheckVersionPolicy checks if image tag update is allowed by policy
func CheckVersionPolicy(currentTag string, newTag string, policy Policy) (bool, error) {
	// Same tag = digest update only, always allow regardless of policy
	if currentTag == newTag {
		return true, nil
	}

	if policy == PolicyAll {
		return true, nil
	}

	currentVer, err := ParseSemVer(currentTag)
	if err != nil {
		logger.Log.Debugf("Tag %s is not semver - allowing update", currentTag)
		return true, nil
	}

	newVer, err := ParseSemVer(newTag)
	if err != nil {
		logger.Log.Debugf("Tag %s is not semver - allowing update", newTag)
		return true, nil
	}

	allowed := IsAllowed(currentVer, newVer, policy)
	if allowed {
		logger.Log.Infof("Version update allowed: %s -> %s (policy: %s)", currentTag, newTag, policy)
	} else {
		logger.Log.Warnf("Version update blocked: %s -> %s (policy: %s)", currentTag, newTag, policy)
	}

	return allowed, nil
}

// MatchesPattern checks if a tag matches a glob pattern
func MatchesPattern(tag string, pattern string) bool {
	regexPattern := "^" + strings.ReplaceAll(pattern, "*", ".*") + "$"
	matched, err := regexp.MatchString(regexPattern, tag)
	if err != nil {
		logger.Log.Warnf("Invalid pattern %s: %v", pattern, err)
		return false
	}
	return matched
}

// GetTagFromImage extracts tag from image name correctly handling host:port refs
// Examples:
//   nginx:latest            -> latest
//   registry:5000/app:v1.2  -> v1.2
//   nginx                   -> latest
func GetTagFromImage(image string) string {
	// Remove digest if present
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}

	// Find last slash - tag colon must be after it
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")

	if lastColon > lastSlash {
		return image[lastColon+1:]
	}

	return "latest"
}