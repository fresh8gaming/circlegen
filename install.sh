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
  echo "Sorry, you'll need to install the circlegen manually."
  exit 1
    ;;
esac

if [[ -z "${TAG}" ]]; then
  tag=$(basename $(curl -fs -o/dev/null -w %{redirect_url} https://github.com/fresh8gaming/circlegen/releases/latest))
else
  tag=${TAG}
fi

filename="circlegen_${tag#v}_${os}"
curl -LO https://github.com/fresh8gaming/circlegen/releases/download/${tag}/${filename}

case $family in
  'linux')
    mkdir -p ~/.local/bin/
    mv ./${filename} ~/.local/bin/circlegen
    chmod +x ~/.local/bin/circlegen
    ;;
  'mac')
    sudo mv ./${filename} /usr/local/bin/circlegen
    chmod +x /usr/local/bin/circlegen
    ;;
  *)
  echo "Sorry, you'll need to move the circlegen binary manually."
  exit 1
    ;;
esac