#!/usr/bin/env bash
# tools/release-downstream.sh
#
# Propagates an lpk release to downstream repos:
#   1. personal-ports  — bumps PORTVERSION, regenerates distinfo from GitHub tarball, commits, pushes
#
# Run from the repo root after the GitHub release has been published.
#
# Required:
#   git, curl, sha256sum, awk
#
# Environment:
#   PERSONAL_PORTS_DIR     — path to personal-ports checkout (default: ~/Code/personal-ports)
#   PERSONAL_PORTS_REMOTE  — override clone URL
#   GIT_AUTHOR_NAME        — overrides git commit author name (useful in CI)
#   GIT_AUTHOR_EMAIL       — overrides git commit author email
#   DRY_RUN                — set to 1 to skip git push

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

WORK_DIR="${WORK_DIR:-$(mktemp -d)}"
[[ -z "${WORK_DIR_EXTERNAL:-}" ]] && trap 'rm -rf "${WORK_DIR}"' EXIT

DRY_RUN="${DRY_RUN:-0}"

if [[ -n "${CI:-}" ]]; then
  PERSONAL_PORTS_DIR="${WORK_DIR}/personal-ports"
else
  PERSONAL_PORTS_DIR="${PERSONAL_PORTS_DIR:-${HOME}/Code/personal-ports}"
fi

PERSONAL_PORTS_REMOTE="${PERSONAL_PORTS_REMOTE:-git@github.com:zachfi/personal-ports.git}"

# ── Version ──────────────────────────────────────────────────────────────────

VERSION="$(git -C "${REPO_ROOT}" describe --tags --exact-match 2>/dev/null || true)"
if [[ -z "${VERSION}" ]]; then
  echo "ERROR: HEAD is not on an exact tag. Tag the commit before running." >&2
  exit 1
fi
VERSION_NO_V="${VERSION#v}"

echo "==> Downstream release: ${VERSION}"
[[ "${DRY_RUN}" == "1" ]] && echo "    (DRY_RUN: git push will be skipped)"

# ── Git identity (CI-friendly) ────────────────────────────────────────────────

if [[ -n "${GIT_AUTHOR_NAME:-}" ]]; then
  git config --global user.name  "${GIT_AUTHOR_NAME}"
  git config --global user.email "${GIT_AUTHOR_EMAIL:-release@lpk}"
fi

if [[ -n "${GITHUB_TOKEN:-}" && -n "${CI:-}" ]]; then
  git config --global \
    url."https://${GITHUB_TOKEN}@github.com/".insteadOf "git@github.com:"
fi

# ── personal-ports ─────────────────────────────────────────────────────────

echo ""
echo "==> [1/1] personal-ports (FreeBSD)"

GH_ACCOUNT="zachfi"
PORT_SUBDIR="sysutils/lpk"

if [[ ! -d "${PERSONAL_PORTS_DIR}/.git" ]]; then
  git clone "${PERSONAL_PORTS_REMOTE}" "${PERSONAL_PORTS_DIR}"
fi

PORT_DIR="${PERSONAL_PORTS_DIR}/${PORT_SUBDIR}"

CURRENT_VER="$(grep '^PORTVERSION' "${PORT_DIR}/Makefile" | awk '{print $NF}')"
if [[ "${CURRENT_VER}" == "${VERSION_NO_V}" ]]; then
  echo "    ${VERSION_NO_V} already set in port Makefile — skipping"
else
  sed -i "s|^PORTVERSION=.*|PORTVERSION=\t${VERSION_NO_V}|" "${PORT_DIR}/Makefile"
  sed -i "s|^PORTREVISION=.*|PORTREVISION=\t0|"             "${PORT_DIR}/Makefile"
  echo "    PORTVERSION → ${VERSION_NO_V}"

  TGZ_FILE="${WORK_DIR}/${GH_ACCOUNT}-lpk-v${VERSION_NO_V}_GH0.tar.gz"
  TGZ_URL="https://github.com/${GH_ACCOUNT}/lpk/archive/refs/tags/v${VERSION_NO_V}.tar.gz"

  echo "--> Fetching ${TGZ_URL}"
  curl -fsSL "${TGZ_URL}" -o "${TGZ_FILE}"

  SHA_TGZ="$(sha256sum "${TGZ_FILE}" | awk '{print $1}')"
  SZ_TGZ="$(wc -c < "${TGZ_FILE}")"
  echo "    sha256=${SHA_TGZ}  size=${SZ_TGZ}"

  cat > "${PORT_DIR}/distinfo" <<EOF
TIMESTAMP = $(date +%s)
SHA256 (${GH_ACCOUNT}-lpk-v${VERSION_NO_V}_GH0.tar.gz) = ${SHA_TGZ}
SIZE (${GH_ACCOUNT}-lpk-v${VERSION_NO_V}_GH0.tar.gz) = ${SZ_TGZ}
EOF

  git -C "${PERSONAL_PORTS_DIR}" add "${PORT_DIR}/Makefile" "${PORT_DIR}/distinfo"
  git -C "${PERSONAL_PORTS_DIR}" commit \
    -m "chore: update ${PORT_SUBDIR} for ${VERSION}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "    DRY_RUN: would push personal-ports"
  else
    git -C "${PERSONAL_PORTS_DIR}" push
    echo "    personal-ports pushed"
  fi
fi

echo ""
echo "==> Downstream release complete for ${VERSION}"
