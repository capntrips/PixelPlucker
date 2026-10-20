# Pixel Plucker

Pixel Plucker is a cli utility that surgically extracts partitions and files out of local or remote Pixel factory images without downloading or extracting the entire archive.

It works by reading zip and erofs structures with HTTP range requests or file seeks then extracting the target file with the same. It supports deflate for extracting the images. It processes the images by reading the avb, fbpack, and tensor bootloader structures.

## Usage

```text
Usage: pluck <command> [arguments]

Commands:
  extract-factory           Extract partition image from factory image
  passthrough-factory       Pass partition image from factory image through to subcommand
  extract-zip               Extract target file from zip file
  extract-erofs             Extract target file from erofs partition image
  extract-bootloader        Extract partition image from factory bootloader image
  list-factory              List factory zip contents
  list-bootloader           List factory bootloader image entries
  list-bootloader-ar        Print anti-rollback value from factory bootloader image entry
  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image
  list-avb-props            List AVB property descriptors
  list-avb-rollback-index   Print AVB rollback index value
  verify-avb-hash           Verify AVB hash descriptor against partition image
  version                   Print version and exit
  help                      Show usage
```

```text
Usage: pluck extract-factory <factoryImageUrlOrFile> <partitionFilename>

Extract partition from factory image

Arguments:
  factoryImageUrlOrFile     Remote URL or local file path of factory image
  partitionFilename         Name of partition image to extract

Use radio.img to extract radio-*.img with its original filename.
```

```text
Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> <subcommand> [arguments]

Pass partition image from factory image through to subcommand

Arguments:
  factoryImageUrlOrFile     Remote URL or local file path of factory image
  partitionFilename         Name of partition image to pass through
  subcommand                Subcommand to handle partition image

Use radio.img to select radio-*.img from the factory archive.

Supported subcommands:
  extract-erofs             Extract target file from erofs partition image
  extract-bootloader        Extract partition image from bootloader image
  list-bootloader           List bootloader image entries
  list-bootloader-ar        Print anti-rollback value from bootloader image entry
  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image
  list-avb-props            List AVB properties
  list-avb-rollback-index   Print AVB rollback index
  help                      Show usage
```

```text
Usage: pluck extract-zip <zipUrlOrFile> <targetFilename>

Extract target file from zip file

Arguments:
  zipUrlOrFile              Remote URL or local file path of zip file
  filePath                  Path to file in zip file to extract
```

```text
Usage: pluck extract-erofs <partitionImageUrlOrFile> <filePath>

Extract target file from erofs partition image

Arguments:
  partitionImageUrlOrFile   Remote URL or local file path of partition image
  filePath                  Path to file in partition image to extract
```

```text
Usage: pluck extract-bootloader <bootloaderImageUrlOrFile> <partitionName>

Extract partition image from factory bootloader image

Arguments:
  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image
  partitionName             Name of partition image to extract
```

```text
Usage: pluck list-factory <factoryImageUrlOrFile>

List factory zip contents

Arguments:
  factoryImageUrlOrFile     Remote URL or local file path of factory image
```

```text
Usage: pluck list-bootloader <bootloaderImageUrlOrFile>

List factory bootloader image entries

Arguments:
  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image
```

```text
Usage: pluck list-bootloader-ar <bootloaderImageUrlOrFile> <partitionName>

Print anti-rollback value from factory bootloader image entry

Arguments:
  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image
  partitionName             Name of partition containing anti-rollback value
```

```text
Usage: pluck list-bootloader-image-ar <partitionImageUrlOrFile>

Print anti-rollback value from bootloader partition image

Arguments:
  partitionImageUrlOrFile   Remote URL or local file/device path of partition image
```

```text
Usage: pluck list-avb-props <partitionImageUrlOrFile>

List AVB property descriptors

Arguments:
  partitionImageUrlOrFile   Remote URL or local file/device path of partition image
```

```text
Usage: pluck list-avb-rollback-index <partitionImageUrlOrFile>

Print AVB rollback index value

Arguments:
  partitionImageUrlOrFile   Remote URL or local file/device path of partition image
```

```text
Usage: pluck verify-avb-hash <vbmetaImageFile> <targetName> <targetPath>

Verify AVB hash descriptor against file/device

Arguments:
  vbmetaImageFile           Local file/device path of vbmeta image
  targetName                Name of partition to verify
  targetPath                Local file/device path of partition to verify
```

`radio.img` selects the versioned `radio-*.img` file in the outer factory ZIP.
Extraction preserves its original filename (without the archive directory), so
you do not need to know the device or radio version ahead of time:

```sh
pluck extract-factory <factoryImageUrlOrFile> radio.img
```

This selects the outer radio image, rather than `modem.img` inside the nested
`image-*.zip`, and prints the saved filename after successful extraction.
The alias is also available in `passthrough-factory`; the selected image must
be compatible with the chosen subcommand. `extract-zip` continues to use the
archive filename directly and does not resolve the `radio.img` alias.
The existing `bootloader.img` alias continues to save as
`bootloader.img`.

### Examples
```bash
pluck list-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip
```
List all the files in the outer factory image zip and the nested `grizzly-cd1a.260905.001.b1/image-grizzly-cd1a.260905.001.b1.zip` file.

5.46 kb in 5 requests.

```bash
pluck extract-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip init_boot.img
```
Extract the `init_boot.img` partition image from the full factory image.

2.25 mb in 7 requests.
```bash
pluck passthrough-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip system.img extract-erofs /system/build.prop
```
Extract the `/system/build.prop` file from the `system.img` partition image in the full factory image.

12.33kb in 16 requests.
```bash
pluck passthrough-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip product.img list-avb-props
```
List the AVB property descriptors from `product.img` partition image in the full factory image.

6.87kb in 10 requests.
```bash
pluck passthrough-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip vbmeta_system.img list-avb-props
```
Extract the `vbmeta_system.img` partition image from the full factory image to a temporary file and list the AVB property descriptors.

11.46kb in 7 requests.
```bash
pluck passthrough-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip vbmeta.img list-avb-rollback-index
```
Extract the `vbmeta.img` partition image from the full factory image to a temporary file and print the AVB rollback index.

15.06kb in 7 requests.
```bash
pluck passthrough-factory https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip bootloader.img list-bootloader-ar gsa_bl1
```
Extract the `gba_bl1.img` bootloader partition image from the full factory image to a temporary file and list the AVB property descriptors.

10.86mb in 7 requests.

