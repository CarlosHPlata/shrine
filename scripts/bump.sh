#!/usr/bin/env bash
#
# Tags HEAD with the next release version and pushes the tag to origin; the tag
# push triggers the Release workflow, which builds the binaries with GoReleaser
# and publishes them as a GitHub release.
#
#   scripts/bump.sh                 checks, menu, confirmation
#   scripts/bump.sh --minor -y      checks only, no questions
#   scripts/bump.sh --minor --beta  first beta of the next minor
#
# All options: scripts/bump.sh --help
set -euo pipefail

readonly RELEASE_BRANCH=main
readonly REMOTE=origin
readonly CI_WORKFLOW=ci.yml   # the workflow in .github/workflows/ that must be green
# vX.Y.Z without leading zeros: bash would read "08" as octal and fail.
readonly STABLE_TAG='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
readonly PRE_TAG='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-(alpha|beta)\.[1-9][0-9]*$'
readonly USAGE='usage: bump.sh [--major | --minor | --patch] [--alpha | --beta] [-y] [--dry-run]'
LEVELS=(major minor patch alpha beta)

usage() {
  cat <<EOF
$USAGE

Tags HEAD with the next release version and pushes the tag to $REMOTE.
Without a level it shows a menu; before pushing it asks for confirmation.

  --major      v1.1.1 → v2.0.0
  --minor      v1.1.1 → v1.2.0
  --patch      v1.1.1 → v1.1.2
  --alpha      v1.1.1 → v1.1.2-alpha.1 → v1.1.2-alpha.2 …
  --beta       v1.1.1 → v1.1.2-beta.1  → v1.1.2-beta.2  …
  -y, --yes    push without asking
  --dry-run    only show the next version: no checks, no tag, no push
  -h, --help   show this help

Before anything is tagged it checks that you are on $RELEASE_BRANCH with no
modified, staged or untracked files, level with $REMOTE/$RELEASE_BRANCH, that
CI ($CI_WORKFLOW, which includes the integration tests) passed for this commit,
and that go test passes.

The base is the highest stable tag (vX.Y.Z). An alpha or beta leads up to the
version already in prerelease, or else to the next patch; add --major, --minor
or --patch to pick it (--minor --beta: v1.1.1 → v1.2.0-beta.1). Releasing that
version later, with the same level, makes the stable release. With no tags yet
it counts from v0.0.0.

Alphas and betas are published as GitHub prereleases, which install.sh and
shrine update skip: only people who ask for that tag get them.
EOF
}

usage_error() {
  echo "error: $*" >&2
  echo "$USAGE   (--help for details)" >&2
  exit 2
}

die() {
  echo "error: $*" >&2
  exit 1
}

check_ok() {
  echo "  ✓ $*"
}

# A pushed tag is a published release, so only tag reviewed, tested code.
# Cheapest checks first; runs before the menu so nobody answers questions for
# a release that can't happen.
preflight() {
  local branch sha ci status conclusion url output
  echo "Checking before release:"

  branch=$(git branch --show-current)
  [[ "$branch" == "$RELEASE_BRANCH" ]] \
    || die "releases are tagged from '$RELEASE_BRANCH', you are on '${branch:-a detached HEAD}'"
  check_ok "on $RELEASE_BRANCH"

  if [[ -n "$(git status --porcelain)" ]]; then
    git status --short >&2
    die "commit, stash or remove the changes above first"
  fi
  check_ok "no modified, staged or untracked files"

  sha=$(git rev-parse HEAD)
  [[ "$sha" == "$(git rev-parse "$REMOTE/$RELEASE_BRANCH")" ]] \
    || die "local $RELEASE_BRANCH differs from $REMOTE/$RELEASE_BRANCH; run 'git pull' (unpushed commits go through a PR)"
  check_ok "up to date with $REMOTE/$RELEASE_BRANCH (${sha:0:7})"

  command -v gh >/dev/null \
    || die "the GitHub CLI is needed to check CI: install it from https://cli.github.com, then run 'gh auth login'"
  # "|" as separator: tabs would collapse when a field (conclusion) is empty.
  ci=$(gh run list --workflow "$CI_WORKFLOW" --commit "$sha" --limit 1 --json status,conclusion,url \
         --jq '.[0] // {} | [.status // "", .conclusion // "", .url // ""] | join("|")') \
    || die "could not read the CI status from GitHub (check 'gh auth status')"
  IFS='|' read -r status conclusion url <<< "$ci"
  case "$status/$conclusion" in
    completed/success) check_ok "CI passed for ${sha:0:7}" ;;
    /) die "no CI run ($CI_WORKFLOW) found for ${sha:0:7}; a release needs a green run on this commit" ;;
    completed/*) die "CI did not pass for ${sha:0:7} ($conclusion): $url" ;;
    *) die "CI is still running for ${sha:0:7} ($status); try again when it's done: $url" ;;
  esac

  command -v go >/dev/null || die "go is not on PATH"
  echo "  … running go test ./..."
  if ! output=$(go test ./... 2>&1); then
    echo "$output" >&2
    die "tests failed; nothing tagged"
  fi
  check_ok "tests pass"
  echo
}

# True when vA.B.C is a higher version than vX.Y.Z.
version_gt() {
  local a1 a2 a3 b1 b2 b3
  IFS=. read -r a1 a2 a3 <<< "${1#v}"
  IFS=. read -r b1 b2 b3 <<< "${2#v}"
  (( a1 > b1 || (a1 == b1 && (a2 > b2 || (a2 == b2 && a3 > b3))) ))
}

# The version an alpha or beta leads up to when no level is given: the one
# already in prerelease (above the latest release), otherwise the next patch.
upcoming_version() {
  local pending
  pending=$(git tag --list 'v*' --sort=-v:refname | grep -E "$PRE_TAG" | head -n1 || true)
  pending="${pending%%-*}"
  if [[ -n "$pending" ]] && version_gt "$pending" "v$major.$minor.$patch"; then
    echo "$pending"
  else
    echo "v$major.$minor.$((patch + 1))"
  fi
}

# next_version LEVEL [PRE]: a stable bump, or with PRE (alpha/beta) the next
# vX.Y.Z-PRE.N of that bump (of upcoming_version when LEVEL is empty).
next_version() {
  local target n
  if [[ -z "${2:-}" ]]; then
    case "$1" in
      major) echo "v$((major + 1)).0.0" ;;
      minor) echo "v$major.$((minor + 1)).0" ;;
      patch) echo "v$major.$minor.$((patch + 1))" ;;
    esac
    return
  fi
  if [[ -n "$1" ]]; then target=$(next_version "$1"); else target=$(upcoming_version); fi
  n=$(git tag --list "$target-$2.*" | grep -E "$PRE_TAG" | sed -E 's/.*\.//' | sort -n | tail -n1 || true)
  echo "$target-$2.$(( ${n:-0} + 1 ))"
}

choose_level() {
  local i choice
  echo
  for i in 1 2 3 4 5; do
    printf '  %d) %-6s → %s\n' "$i" "${LEVELS[i-1]}" "$(menu_version "${LEVELS[i-1]}")"
  done
  echo
  while true; do
    printf 'Which version? [1-5]: '
    read -r choice || { echo; die "aborted: no version chosen"; }
    case "$choice" in
      [1-5]) choice="${LEVELS[choice-1]}" ;;
      major|minor|patch|alpha|beta) ;;
      *) echo "Please answer 1-5."; continue ;;
    esac
    case "$choice" in
      alpha|beta) pre="$choice" ;;
      *) level="$choice" ;;
    esac
    return
  done
}

menu_version() {
  case "$1" in
    alpha|beta) next_version "" "$1" ;;
    *) next_version "$1" ;;
  esac
}

confirm() {
  local answer
  printf 'Push %s to %s? This publishes a release. [y/N]: ' "$next" "$REMOTE"
  read -r answer || { echo; answer=""; }   # no terminal / Ctrl-D counts as "no"
  case "$answer" in
    y|Y|yes|Yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

level=""
pre=""
assume_yes=false
dry_run=false
for arg in "$@"; do
  case "$arg" in
    --major|--minor|--patch)
      [[ -z "$level" ]] || usage_error "choose one version level, got --$level and $arg"
      level="${arg#--}" ;;
    --alpha|--beta)
      [[ -z "$pre" ]] || usage_error "choose alpha or beta, got --$pre and $arg"
      pre="${arg#--}" ;;
    -y|--yes) assume_yes=true ;;
    --dry-run) dry_run=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage_error "unknown option '$arg'" ;;
  esac
done

# go test is relative to the repo root, wherever this was run from.
cd "$(git rev-parse --show-toplevel)"

# Local tags are only as fresh as the last fetch; this also updates origin/main.
git fetch --quiet --tags "$REMOTE"

$dry_run || preflight

# `|| true`: with no tags grep matches nothing and exits 1, which pipefail would turn into an exit.
latest=$(git tag --list 'v*' --sort=-v:refname | grep -E "$STABLE_TAG" | head -n1 || true)
IFS=. read -r major minor patch <<< "${latest:-v0.0.0}"
major="${major#v}"

echo "Latest release: ${latest:-none}"
[[ -n "$level" || -n "$pre" ]] || choose_level
next=$(next_version "$level" "$pre")
echo "Next version:   $next (${level}${level:+${pre:+ }}${pre})"

if $dry_run; then
  echo "Dry run: nothing tagged or pushed."
  exit 0
fi

if [[ -z "$pre" ]]; then
  # Running a stable bump twice would release the same code under two versions.
  released=$(git tag --points-at HEAD | grep -E "$STABLE_TAG" | head -n1 || true)
  [[ -z "$released" ]] || die "this commit is already released as $released"
elif [[ "$pre" == alpha && -n "$(git tag --list "${next%%-*}-beta.*")" ]]; then
  # alpha sorts before beta, so an alpha after the first beta would go backwards.
  die "${next%%-*} is already in beta; release another beta instead"
fi
if git rev-parse --quiet --verify "refs/tags/$next" >/dev/null; then
  die "tag $next already exists"
fi

$assume_yes || confirm || { echo "Aborted: nothing tagged or pushed."; exit 1; }

# Until the push succeeds, any exit (failure, Ctrl-C) removes the local tag;
# otherwise the next run would count a version that never reached GitHub.
trap 'git tag --delete "$next" >/dev/null 2>&1 || true' EXIT
trap 'exit 130' INT TERM
git tag --annotate "$next" --message "Release $next"
git push --quiet "$REMOTE" "refs/tags/$next" || die "push failed; removed the local tag $next"
trap - EXIT

echo "Pushed $next; the Release workflow is building and publishing it."
