#!/bin/bash
# Check for tools required by v

OK=0
WARN=0

check() {
    if command -v "$1" &>/dev/null; then
        printf "  %-24s \e[32mfound\e[0m  (%s)\n" "$1" "$(command -v "$1")"
    else
        printf "  %-24s \e[31mmissing\e[0m\n" "$1"
        if [ "$2" = "required" ]; then
            OK=1
        else
            WARN=1
        fi
    fi
}

echo "Required:"
check qemu-system-x86_64   required
check qemu-img              required
check ip                    required
check npm                  required

echo ""
echo "Required (one of):"
if command -v genisoimage &>/dev/null; then
    printf "  %-24s \e[32mfound\e[0m  (%s)\n" "genisoimage (cdrtools)" "$(command -v genisoimage)"
elif command -v mkisofs &>/dev/null; then
    printf "  %-24s \e[32mfound\e[0m  (%s)\n" "mkisofs" "$(command -v mkisofs)"
else
    printf "  %-24s \e[31mmissing\e[0m\n" "genisoimage (cdrtools) / mkisofs"
    OK=1
fi

echo ""
echo "KVM support:"
if [ -e /dev/kvm ]; then
    if [ -r /dev/kvm ] && [ -w /dev/kvm ]; then
        printf "  %-24s \e[32mavailable\e[0m\n" "/dev/kvm"
    else
        printf "  %-24s \e[33mno access\e[0m (add user to kvm group)\n" "/dev/kvm"
        WARN=1
    fi
else
    printf "  %-24s \e[31mnot found\e[0m (enable KVM in BIOS/kernel)\n" "/dev/kvm"
    OK=1
fi

echo ""
echo "Optional (bridge networking):"
check iptables              optional
check dnsmasq               optional

echo ""
if [ $OK -ne 0 ]; then
    echo -e "\e[31mSome required dependencies are missing.\e[0m"
    exit 1
elif [ $WARN -ne 0 ]; then
    echo -e "\e[33mAll required deps found. Some optional deps missing.\e[0m"
else
    echo -e "\e[32mAll dependencies found.\e[0m"
fi
