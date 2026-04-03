#!/bin/bash

cd "$(dirname "$0")/.."

read -n1 -p "This will use npm install to download xterm. Continue (Y/n)? " choice
case "$choice" in 
  n|N ) echo "" && exit 1;;
  * ) echo "";;
esac

npm install
npm run postinstall
