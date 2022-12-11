# Use Bash shell.
# NOTE: You can control which Bash version is used by setting the PATH
# appropriately.
SHELL := bash

# Path to the directory of this Makefile.
# NOTE: Prepend all relative paths in this Makefile with this variable to ensure
# they are properly resolved when this Makefile is included from Makefiles in
# other directories.
SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

# Function for comparing two strings for equality.
# NOTE: This will also return false if both strings are empty.
eq = $(and $(findstring $(1),$(2)), $(findstring $(2),$(1)))

# Check if we're running in an interactive terminal.
ISATTY := $(shell [ -t 0 ] && echo 1)

# If running interactively, use terminal colors.
ifdef ISATTY
	MAGENTA := \e[35;1m
	CYAN := \e[36;1m
	RED := \e[0;31m
	OFF := \e[0m
	ECHO_CMD := echo -e
else
	MAGENTA := ""
	CYAN := ""
	RED := ""
	OFF := ""
	ECHO_CMD := echo
endif

# Output messages to stderr instead stdout.
ECHO := $(ECHO_CMD) 1>&2

# Boolean indicating whether to assume the 'yes' answer when confirming actions.
ASSUME_YES ?= 0

# Helper that asks the user to confirm the action.
define CONFIRM_ACTION =
	if [[ $(ASSUME_YES) != 1 ]]; then \
		$(ECHO) -n "Are you sure? [y/N] " && read ans && [ $${ans:-N} = y ]; \
	fi
endef

# Name of git remote pointing to the canonical upstream git repository, i.e.
# git@github.com:oasisprotocol/<repo-name>.git.
GIT_ORIGIN_REMOTE ?= origin

# Name of the branch where to tag the next release.
RELEASE_BRANCH ?= main

# Determine project's version from git.
# NOTE: This computes the project's version from the latest version tag
# reachable from current git commit and does not search for version
# tags across the whole $(GIT_ORIGIN_REMOTE) repository.
GIT_VERSION := $(subst v,,$(shell \
	git describe --tags --match 'v*' --abbrev=0 2>/dev/null HEAD || \
	echo undefined \
))

# Determine project's version.
# If the current git commit is exactly a tag and it equals the Punch version,
# then the project's version is that.
# Else, the project version is the Punch version appended with git commit and
# dirty state info.
GIT_COMMIT_EXACT_TAG := $(shell \
	git describe --tags --match 'v*' --exact-match &>/dev/null HEAD && echo YES || echo NO \
)

# Go binary to use for all Go commands.
export OASIS_GO ?= go

# Go command prefix to use in all Go commands.
GO := env -u GOPATH $(OASIS_GO)

# NOTE: The -trimpath flag strips all host dependent filesystem paths from
# binaries which is required for deterministic builds.
GOFLAGS ?= -trimpath -v

VERSION := $(or \
	$(and $(call eq,$(GIT_COMMIT_EXACT_TAG),YES), $(GIT_VERSION)), \
	$(shell git describe --tags --abbrev=0)-git$(shell git describe --always --match '' --dirty=+dirty 2>/dev/null) \
)

# Project's version as the linker's string value definition.
export GOLDFLAGS_VERSION := -X github.com/oasisprotocol/oasis-web3-gateway/version.Software=$(VERSION)

# Go's linker flags.
export GOLDFLAGS ?= "$(GOLDFLAGS_VERSION)"

# Helper that ensures the git workspace is clean.
define ENSURE_GIT_CLEAN =
	if [[ ! -z `git status --porcelain` ]]; then \
		$(ECHO) "$(RED)Error: Git workspace is dirty.$(OFF)"; \
		exit 1; \
	fi
endef

# Helper that checks if the go mod tidy command was run.
# NOTE: go mod tidy doesn't implement a check mode yet.
# For more details, see: https://github.com/golang/go/issues/27005.
define CHECK_GO_MOD_TIDY =
    $(GO) mod tidy; \
    if [[ ! -z `git status --porcelain go.mod go.sum` ]]; then \
        $(ECHO) "$(RED)Error: The following changes detected after running 'go mod tidy':$(OFF)"; \
        git diff go.mod go.sum; \
        exit 1; \
    fi
endef

# Helper that checks commits with gitlilnt.
# NOTE: gitlint internally uses git rev-list, where A..B is asymmetric
# difference, which is kind of the opposite of how git diff interprets
# A..B vs A...B.
define CHECK_GITLINT =
	BRANCH=$(GIT_ORIGIN_REMOTE)/$(RELEASE_BRANCH); \
	COMMIT_SHA=`git rev-parse $$BRANCH` && \
	$(ECHO) "$(CYAN)*** Running gitlint for commits from $$BRANCH ($${COMMIT_SHA:0:7})... $(OFF)"; \
	gitlint --commits $$BRANCH..HEAD
endef

define newline


endef
