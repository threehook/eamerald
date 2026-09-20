package cc

import (
	"os"
	"strconv"
	"time"

	"github.com/threehook/eamerald/mrld/constants"
)

const (
	defaultDirectorySvc    = "localhost:9292"
	defaultDirectoryKey    = ""
	defaultDirectoryToken  = ""
	defaultAuthorizerSvc   = "localhost:8282"
	defaultAuthorizerKey   = ""
	defaultAuthorizerToken = ""
	defaultInsecure        = false
	defaultPlaintext       = false
	defaultTimeout         = 5 * time.Second
)

func DirectorySvc() string {
	if directorySvc := os.Getenv(constants.EnvEameraldDirectorySvc); directorySvc != "" {
		return directorySvc
	}

	return defaultDirectorySvc
}

func DirectoryKey() string {
	if directoryKey := os.Getenv(constants.EnvEameraldDirectoryKey); directoryKey != "" {
		return directoryKey
	}

	return defaultDirectoryKey
}

func DirectoryToken() string {
	if directoryToken := os.Getenv(constants.EnvEameraldDirectoryToken); directoryToken != "" {
		return directoryToken
	}

	return defaultDirectoryToken
}

func AuthorizerSvc() string {
	if authorizerSvc := os.Getenv(constants.EnvEameraldAuthorizerSvc); authorizerSvc != "" {
		return authorizerSvc
	}

	return defaultAuthorizerSvc
}

func AuthorizerKey() string {
	if authorizerKey := os.Getenv(constants.EnvEameraldAuthorizerKey); authorizerKey != "" {
		return authorizerKey
	}

	return defaultAuthorizerKey
}

func AuthorizerToken() string {
	if authorizerToken := os.Getenv(constants.EnvEameraldAuthorizerToken); authorizerToken != "" {
		return authorizerToken
	}

	return defaultAuthorizerToken
}

func Insecure() bool {
	if insecure := os.Getenv(constants.EnvEameraldInsecure); insecure != "" {
		if b, err := strconv.ParseBool(insecure); err == nil {
			return b
		}
	}

	return defaultInsecure
}

func Plaintext() bool {
	if plaintext := os.Getenv(constants.EnvEameraldPlaintext); plaintext != "" {
		if b, err := strconv.ParseBool(plaintext); err == nil {
			return b
		}
	}

	return defaultPlaintext
}

func Timeout() time.Duration {
	if timeout := os.Getenv(constants.EnvEameraldTimeout); timeout != "" {
		if dur, err := time.ParseDuration(timeout); err == nil {
			return dur
		}
	}

	return defaultTimeout
}

func NoCheck() bool {
	if noCheck := os.Getenv(constants.EnvEameraldNoCheck); noCheck != "" {
		if b, err := strconv.ParseBool(noCheck); err == nil {
			return b
		}
	}

	return defaults.NoCheck
}

func NoColor() bool {
	if noColor := os.Getenv(constants.EnvEameraldNoColor); noColor != "" {
		if b, err := strconv.ParseBool(noColor); err == nil {
			return b
		}
	}

	return defaults.NoColor
}
