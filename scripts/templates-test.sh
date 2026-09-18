#!/usr/bin/env bash

templates=("assets/acmecorp.json" "assets/api-auth.json" "assets/api-gateway.json" "assets/citadel.json" "assets/gdrive.json" "assets/github.json" "assets/multi-tenant.json" "assets/peoplefinder.json" "assets/simple-rbac.json" "assets/slack.json" "assets/todo.json")

tmrld="./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld"

eval "$tmrld version"

for tmpl in ${templates[@]}; do
  echo $tmpl
  # cat $tmpl | jq .

  args="directory delete manifest --force --plaintext"
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  manifest=$(cat $tmpl | jq -r '.assets.manifest')
  echo $manifest
  args="directory set manifest $PWD/assets/$manifest --plaintext"
  echo $args
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  idp_data=$(cat $tmpl | jq -r '.assets.idp_data[0]')
  idp_data_dir=$(dirname "$idp_data" )
  echo $idp_data_dir
  args="directory import --directory $PWD/assets/$idp_data_dir --plaintext"
  echo $args
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  domain_data=$(cat $tmpl | jq -r '.assets.domain_data[0]')
  domain_data_dir=$(dirname "$domain_data" )
  echo $domain_data_dir
  if [[ -z "$domain_data" ]]; then
    echo "NO DOMAIN DATA"
  else
    args="directory import --directory $PWD/assets/$domain_data_dir --plaintext"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi

  assertion=$(cat $tmpl | jq -r '.assets.assertions[0]')
  echo $assertion
  if [[ -z "$assertion" ]]; then
    echo "NO ASSERTIONS"
  else
    args="directory test exec $PWD/assets/$assertion --summary --plaintext"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi

  decisions=$(cat $tmpl | jq -r '.assets.assertions[1]')
  echo $decisions
  if [[ -z "$decisions" ]]; then
    echo "NO DECISIONS"
  else
    args="authorizer test exec $PWD/assets/$decisions --summary --plaintext --host localhost:9292"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi
done
