package updater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sentinel/logger"
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
	PolicyAll   Policy = "all"   // allow all updates
	PolicyMajor Policy = "major" // allow major, minor, patch
	PolicyMinor Policy = "minor" // allow minor and patch only
	PolicyPatch Policy = "patch" // allow patch only
	PolicyNone  Policy = "none"  // no updates allowed
)

// ParseSemVer parses a version string into SemVer
// Supports: 1.2.3 or v1.2.3 or 1.2 or 1
func ParseSemVer(version string) (*SemVer, error) {
	// Remove v prefix if exists
	version = strings.TrimPrefix(version, "v")

	// Split by dot
	parts := strings.Split(version, ".")

	// Need at least major version
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid version: %s", version)
	}

	sv := &SemVer{Raw: version}

	// Parse major
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", parts[0])
	}
	sv.Major = major

	// Parse minor if exists
	if len(parts) > 1 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid minor version: %s", parts[1])
		}
		sv.Minor = minor
	}

	// Parse patch if exists
	if len(parts) > 2 {
		// Remove any suffix like -alpine or -beta
		patchStr := strings.Split(parts[2], "-")[0]
		patch, err := strconv.Atoi(patchStr)
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", parts[2])
		}
		sv.Patch = patch
	}

	return sv, nil
}

// IsNewer checks if newVer is newer than currentVer
func IsNewer(current *SemVer, new *SemVer) bool {
	// Check major
	if new.Major > current.Major {
		return true
	}
	if new.Major < current.Major {
		return false
	}

	// Major is equal - check minor
	if new.Minor > current.Minor {
		return true
	}
	if new.Minor < current.Minor {
		return false
	}

	// Minor is equal - check patch
	return new.Patch > current.Patch
}

// IsAllowed checks if an update is allowed by policy
func IsAllowed(current *SemVer, new *SemVer, policy Policy) bool {
	switch policy {

	case PolicyNone:
		// No updates allowed
		logger.Log.Debugf("Policy: none - blocking update")
		return false

	case PolicyAll:
		// All updates allowed
		logger.Log.Debugf("Policy: all - allowing update")
		return IsNewer(current, new)

	case PolicyMajor:
		// Allow any version bump
		logger.Log.Debugf("Policy: major - allowing major updates")
		return IsNewer(current, new)

	case PolicyMinor:
		// Block major version bumps
		if new.Major > current.Major {
			logger.Log.Warnf("Policy: minor - blocking major update %s -> %s",
				current.Raw, new.Raw)
			return false
		}
		return IsNewer(current, new)

	case PolicyPatch:
		// Block major and minor version bumps
		if new.Major > current.Major {
			logger.Log.Warnf("Policy: patch - blocking major update %s -> %s",
				current.Raw, new.Raw)
			return false
		}
		if new.Minor > current.Minor {
			logger.Log.Warnf("Policy: patch - blocking minor update %s -> %s",
				current.Raw, new.Raw)
			return false
		}
		return IsNewer(current, new)

	default:
		// Unknown policy - allow all
		logger.Log.Warnf("Unknown policy: %s - allowing all updates", policy)
		return IsNewer(current, new)
	}
}

// CheckVersionPolicy checks if image tag update is allowed
func CheckVersionPolicy(currentTag string, newTag string, policy Policy) (bool, error) {
	// If policy is all or tags are not semver just allow
	if policy == PolicyAll {
		return true, nil
	}

	// Try to parse both tags as semver
	currentVer, err := ParseSemVer(currentTag)
	if err != nil {
		// Not semver - allow update
		logger.Log.Debugf("Tag %s is not semver - allowing update", currentTag)
		return true, nil
	}

	newVer, err := ParseSemVer(newTag)
	if err != nil {
		// Not semver - allow update
		logger.Log.Debugf("Tag %s is not semver - allowing update", newTag)
		return true, nil
	}

	// Check policy
	allowed := IsAllowed(currentVer, newVer, policy)

	if allowed {
		logger.Log.Infof("Version update allowed: %s -> %s (policy: %s)",
			currentTag, newTag, policy)
	} else {
		logger.Log.Warnf("Version update blocked: %s -> %s (policy: %s)",
			currentTag, newTag, policy)
	}

	return allowed, nil
}

// MatchesPattern checks if a tag matches a regex pattern
// Example: pattern = "stable-*" tag = "stable-1.2.3"
func MatchesPattern(tag string, pattern string) bool {
	// Convert glob pattern to regex
	regexPattern := "^" + strings.ReplaceAll(pattern, "*", ".*") + "$"

	matched, err := regexp.MatchString(regexPattern, tag)
	if err != nil {
		logger.Log.Warnf("Invalid pattern %s: %v", pattern, err)
		return false
	}

	return matched
}

// GetTagFromImage extracts tag from image name
// Example: postgres:17-alpine -> 17-alpine
func GetTagFromImage(image string) string {
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return "latest"
	}
	return parts[1]
}