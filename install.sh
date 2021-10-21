#!/bin/bash
case $(uname -sm) in
  'Linux x86_64')
    os='linux_amd64'
    family='linux'
    ;;
  'Darwin x86' | 'Darwin x86_64')
    os='darwin_amd64'
    family='mac'
    ;;
  'Darwin arm64')
    os='darwin_arm64'
    family='mac'
    ;;
  *)
  echo "Sorry, you'll need to install the circleci-config-generator manually."
  exit 1
    ;;
esac

if [[ -z "${TAG}" ]]; then
  tag=$(basename $(curl -fs -o/dev/null -w %{redirect_url} https://github.com/fresh8gaming/circleci-config-generator/releases/latest))
else
  tag=${TAG}
fi

filename="circleci-config-generator_${tag#v}_${os}"
curl -LO https://github.com/fresh8gaming/circleci-config-generator/releases/download/${tag}/${filename}

case $family in
  'linux')
    mv ./${filename} ~/.local/bin
    ;;
  'mac')
    sudo mv ./${filename} /usr/local/bin
    ;;
  *)
  echo "Sorry, you'll need to move the circleci-config-generator binary manually."
  exit 1
    ;;
esac