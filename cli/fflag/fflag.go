package fflag

import (
	"os"
	"strconv"
	"sync"

	"github.com/threehook/eamerald/cli/constants"
)

// feature flags package.
const (
	Default FFlag = 0
)

type FFlag uint64

const (
	Editor FFlag = 1 << iota
)

var (
	ffOnce sync.Once
	ff     FFlag
)

func Init() {
	ffOnce.Do(func() {
		env := os.Getenv(constants.EnvEameraldFeatureFlag)
		if env == "" {
			ff = Default
		}

		f, err := strconv.ParseUint(os.Getenv(constants.EnvEameraldFeatureFlag), 10, 8)
		if err != nil {
			ff = Default
		}

		ff = FFlag(f)
	})
}

func FF() FFlag {
	return ff
}

func Enabled(flag FFlag) bool {
	return ff&flag != 0
}

func (f FFlag) IsSet(flag FFlag) bool {
	return f&flag != 0
}
