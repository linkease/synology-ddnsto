#!/bin/sh

DDNSTO_PATH=/projects/workspace-go/src/github.com/linkease/ddns-client/cmd/cli_ddnsto/release
export X86_BIN=${DDNSTO_PATH}/synology.amd64
export ARM_BIN=${DDNSTO_PATH}/synology.arm

VER=`cat ${DDNSTO_PATH}/latest`
echo "latest version is:" ${VER}

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

