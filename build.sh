#!/bin/sh

export X86_BIN=/projects/gogs-ks/ddnsto-c/build-x86/ddnsto
export ARM_BIN=/projects/gogs-ks/ddnsto-c/build-arm/ddnsto

./pkg.sh ddnsto_arm dsm6
./pkg.sh ddnsto_arm dsm7
./pkg.sh ddnsto_x86 dsm6
./pkg.sh ddnsto_x86 dsm7

cp ddnsto_arm-dsm6.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_arm-dsm7.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_x86-dsm6.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_x86-dsm7.spk /home/janson/Downloads/honor/ddnsto/synology/

