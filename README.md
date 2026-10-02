# Pixel Plucker

Pixel Plucker is a cli utility that surgically extracts partitions and files out of local or remote Pixel factory images without downloading or extracting the entire archive.

It works by reading zip, erofs, and avb structures with HTTP range requests or file seeks then extracting the target file with the same. It supports deflate for extracting the images but will not extract files from within compressed images.

## Usage

```text
Usage: ./pluck [flags] <factoryImageUrlOrFile> [partitionFilename [filePath]]

Arguments:
  factoryImageUrlOrFile    Remote URL or local file path of the factory image
  partitionFilename        Name of the partition to extract (optional)
  filePath                 Path to file in partition to extract (optional)

Flags:
  -v, --version            Print version and exit
  -l, --list               List filenames
  -a, --avb                List AVB props
  -i, --img                Work with partition images directly
```

```text
Usage: ./pluck --img [flags] <partitionImageUrlOrFile> [filePath]

Arguments:
  partitionImageUrlOrFile  Remote URL or local file path of the partition image
  filePath                 Path to file in partition to extract (optional)

Flags:
  -a, --avb                List AVB props
```

### Examples
```bash
pluck --list https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip
```
List all the files in the outer factory image zip and the nested `grizzly-cd1a.260905.001.b1/image-grizzly-cd1a.260905.001.b1.zip` file.

5.46 kb in 5 requests.

```bash
pluck https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip init_boot.img
```
Extract the `init_boot.img` partition image from the full factory image.

2.25 mb in 7 requests.
```bash
pluck https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip system.img /system/build.prop
```
Extract the `/system/build.prop` file from the `system.img` partition image in the full factory image.

12.33kb in 16 requests.
```bash
pluck --avb https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip product.img
```
List the AVB property descriptors from `product.img` partition image in the full factory image.

6.87kb in 10 requests.
```bash
pluck --avb https://dl.google.com/dl/android/aosp/grizzly-cd1a.260905.001.b1-factory-4ce23ec8.zip vbmeta_system.img
```
Extract the `vbmeta_system.img` partition image from the full factory image to a temporary file and list the AVB property descriptors.

11.46kb in 7 requests.

## Note

Files can only be extracted from erofs partition images. Only Pixel 11 factory images have been observed to use erofs partition images.