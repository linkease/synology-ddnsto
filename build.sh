#!/bin/sh

export X86_BIN=/projects/gogs-ks/ddnsto-c/build-x86/ddnsto
export ARM_BIN=/projects/gogs-ks/ddnsto-c/build-arm/ddnsto

./pkg.sh ddnsto_arm 6.0
./pkg.sh ddnsto_arm 7.0
./pkg.sh ddnsto_x86 6.0
./pkg.sh ddnsto_x86 7.0

cp ddnsto_x86.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_arm.spk /home/janson/Downloads/honor/ddnsto/synology/

