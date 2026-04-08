#!/bin/bash

if [[ ! "$(ip link show v-br0 2>/dev/null)" ]]; then
    echo "./v net setup"
    sudo ./v net setup
fi

echo "./v serve"
sudo ./v serve
