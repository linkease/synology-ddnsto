#!/bin/sh

VER=3.0.0

#export X86_BIN=/projects/gogs-ks/ddnsto-c/build-x86/ddnsto
#export ARM_BIN=/projects/gogs-ks/ddnsto-c/build-arm/ddnsto
export X86_BIN=/home/janson/Downloads/honor/ddnsto/ddnsto-binary-${VER}/ddnsto.x86_64
export ARM_BIN=/home/janson/Downloads/honor/ddnsto/ddnsto-binary-${VER}/ddnsto.arm

sed -i "s/version=\".*\"/version=\"${VER}\"/g" arm/INFO-dsm6
sed -i "s/version=\".*\"/version=\"${VER}\"/g" arm/INFO-dsm7
sed -i "s/version=\".*\"/version=\"${VER}\"/g" x86/INFO-dsm6
sed -i "s/version=\".*\"/version=\"${VER}\"/g" x86/INFO-dsm7

./pkg.sh ddnsto_arm dsm6
./pkg.sh ddnsto_arm dsm7
./pkg.sh ddnsto_x86 dsm6
./pkg.sh ddnsto_x86 dsm7

cp ddnsto_arm-dsm6.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_arm-dsm7.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_x86-dsm6.spk /home/janson/Downloads/honor/ddnsto/synology/
cp ddnsto_x86-dsm7.spk /home/janson/Downloads/honor/ddnsto/synology/

