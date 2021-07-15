#!/bin/sh
BUILDDIR=$(cd `dirname $0`;pwd)

# export X86_BIN=x86_bin/synology.amd64
# export ARM_BIN=xxx

VAR=$1
ARCH=
case "$VAR" in
    *_x86)
        ARCH=x86
        cp $BUILDDIR/x86/INFO $BUILDDIR/
        cp $X86_BIN $BUILDDIR/package/bin/ddnsto
        ;;
    *_arm)
        ARCH=arm
        cp $BUILDDIR/arm/INFO $BUILDDIR/
        cp $ARM_BIN $BUILDDIR/package/bin/ddnsto
        ;;
    *)
        echo "not support arch, usage: ./pkg.sh ddnsto_x86"
        exit 1
        ;;
esac


chmod 755 $BUILDDIR/package/bin/ddnsto
echo "arch is" $ARCH
echo "1.清理文件夹"
rm -rf $VAR
rm -rf *.tgz

echo "2.生成package.tgz"
cd $BUILDDIR/package
tar -czf $BUILDDIR/package.tgz *

echo "3.生成$1.SPK"
cd $BUILDDIR
tar -cf $BUILDDIR/$1.spk package.tgz scripts WIZARD_UIFILES CHANGELOG INFO LICENSE PACKAGE_ICON.PNG conf
