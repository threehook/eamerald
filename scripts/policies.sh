#!/usr/bin/env bash
# Builds the Rego policies in assets/policies into OPA bundles (dist/policies/policy-<name>.tar.gz) and publishes them as OCI artifacts.
#   scripts/policies.sh build
#   scripts/policies.sh publish     (needs a token with write:packages: `gh auth refresh -s write:packages`)
set -euo pipefail

REGISTRY="${POLICY_REGISTRY:-ghcr.io/threehook}"
TAG="${POLICY_TAG:-latest}"
SOURCE_URL="${POLICY_SOURCE_URL:-https://github.com/threehook/eamerald}"
OPA_IMAGE="openpolicyagent/opa:latest"
ORAS_IMAGE="ghcr.io/oras-project/oras:v1.2.3"
SRC="$(pwd)/assets/policies"
OUT="$(pwd)/dist/policies"

build() {
	mkdir -p "${OUT}"
	# the policies call the directory built-ins that eameraldd registers, so the compiler needs to know about them.
	docker run --rm "${OPA_IMAGE}" capabilities --current |
		sed 's#"builtins": \[#"builtins": [{"name":"ds.check","decl":{"args":[{"type":"any"}],"result":{"type":"boolean"},"type":"function"}},#' >"${OUT}/capabilities.json"

	for dir in "${SRC}"/*/; do
		name="$(basename "${dir}")"
		docker run --rm -v "${dir}:/src:ro" -v "${OUT}:/out" "${OPA_IMAGE}" \
			build -b /src --capabilities /out/capabilities.json -o "/out/policy-${name}.tar.gz"
		echo "built ${OUT}/policy-${name}.tar.gz"
	done
}

publish() {
	build
	for bundle in "${OUT}"/policy-*.tar.gz; do
		name="$(basename "${bundle}" .tar.gz)"
		gh auth token | docker run --rm -i -v "${OUT}:/w" -w /w "${ORAS_IMAGE}" push "${REGISTRY}/${name}:${TAG}" \
			-u "$(gh api user -q .login)" --password-stdin \
			--annotation "org.opencontainers.image.source=${SOURCE_URL}" \
			"$(basename "${bundle}"):application/vnd.oci.image.layer.v1.tar+gzip"
	done
}

case "${1:-}" in
build) build ;;
publish) publish ;;
*)
	echo "usage: $0 build|publish" >&2
	exit 1
	;;
esac
