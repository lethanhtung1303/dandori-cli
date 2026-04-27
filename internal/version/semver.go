package version

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseSemver(s string) (major, minor, patch int, err error) {
	v := strings.TrimPrefix(s, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid semver %q: expected MAJOR.MINOR.PATCH", s)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major in %q: %w", s, err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor in %q: %w", s, err)
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch in %q: %w", s, err)
	}

	if major < 0 || minor < 0 || patch < 0 {
		return 0, 0, 0, fmt.Errorf("invalid semver %q: negative components", s)
	}

	return major, minor, patch, nil
}
