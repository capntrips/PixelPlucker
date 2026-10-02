package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// https://docs.fileformat.com/compression/zip/

type LocalFileHeader struct {
	Magic             [4]byte // PK\x03\x04
	VersionNeeded     uint16
	GeneralPurpose    uint16
	CompressionMethod uint16
	LastModTime       uint16
	LastModDate       uint16
	CRC32             uint32
	CompressedSize    uint32
	UncompressedSize  uint32
	FilenameLen       uint16
	ExtraFieldLen     uint16
}

type CdFileHeader struct {
	Magic             [4]byte // PK\x01\x02
	VersionMadeBy     uint16
	VersionNeeded     uint16
	GeneralPurpose    uint16
	CompressionMethod uint16
	LastModTime       uint16
	LastModDate       uint16
	CRC32             uint32
	CompressedSize    uint32
	UncompressedSize  uint32
	FilenameLen       uint16
	ExtraFieldLen     uint16
	FileCommentLen    uint16
	DiskNumberStart   uint16
	InternalAttrs     uint16
	ExternalAttrs     uint32
	LfhOffset         uint32
}

type EocdRecord struct {
	Magic        [4]byte // PK\x05\x06
	DiskNumber   uint16
	DiskWithCd   uint16
	DiskRecords  uint16
	TotalRecords uint16
	CdSize       uint32
	CdOffset     uint32
	CommentLen   uint16
}

// https://pkware.cachefly.net/webdocs/APPNOTE/APPNOTE-6.3.1.TXT

type Zip64EocdLocator struct {
	Magic        [4]byte // PK\x06\x07
	DiskWithEocd uint32
	EocdOffset   uint64
	TotalDisks   uint32
}

type Zip64EocdRecord struct {
	Magic         [4]byte // PK\x06\x06
	RecordSize    uint64
	VersionMadeBy uint16
	VersionNeeded uint16
	DiskNumber    uint32
	DiskWithCd    uint32
	DiskRecords   uint64
	TotalRecords  uint64
	CdSize        uint64
	CdOffset      uint64
}

type SparseStub struct {
	Magic [4]byte // 0xed26ff3a
}

type ErofsStub struct {
	Magic [4]byte // 0xe0f5e1e2
}

// https://github.com/erofs/go-erofs/blob/v0.3.1/internal/disk/types.go

type ErofsSuperblock struct {
	Magic            [4]byte // 0xe0f5e1e2
	Checksum         uint32
	FeatureCompat    uint32
	BlkSizeBits      uint8
	ExtSlots         uint8
	RootNid          uint16
	Inos             uint64
	BuildTime        uint64
	BuildTimeNs      uint32
	Blocks           uint32
	MetaBlkAddr      uint32
	XattrBlkAddr     uint32
	UUID             [16]byte
	VolumeName       [16]byte
	FeatureIncompat  uint32
	ComprAlgs        uint16
	ExtraDevices     uint16
	DevtSlotOff      uint16
	DirBlkBits       uint8
	XattrPrefixCount uint8
	XattrPrefixStart uint32
	PackedNid        uint64
	XattrFilterRes   uint8
	Reserved         [23]byte
}

type ErofsInodeChunkInfo struct {
	Format   uint16
	Reserved uint16
}

type ErofsInodeCompact struct {
	Format     uint16
	XattrCount uint16
	Mode       uint16
	Nlink      uint16
	Size       uint32
	Reserved   uint32
	ChunkInfo  ErofsInodeChunkInfo
	Inode      uint32
	UID        uint16
	GID        uint16
	Reserved2  uint32
}

type ErofsInodeExtended struct {
	Format     uint16
	XattrCount uint16
	Mode       uint16
	Reserved   uint16
	Size       uint64
	ChunkInfo  ErofsInodeChunkInfo
	Inode      uint32
	Uid        uint32
	Gid        uint32
	Mtime      uint64
	MtimeNs    uint32
	Nlink      uint32
}

type ErofsXattrIbodyHeader struct {
	NameFilter  uint32
	SharedCount uint8
	Reserved    [7]byte
	// SharedXattrs [XattrCount]uint32
}

//goland:noinspection GoUnusedExportedType
type ErofsXattrEntry struct {
	NameLen   uint8
	NameIndex uint8
	ValueSize uint16
	// Name      [NameLen]byte
}

type ErofsDirent struct {
	Nid      uint64
	NameOff  uint16
	FileType uint8
	Reserved uint8
}

type ErofsInodeChunkIndex struct {
	StartBlk uint32
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_footer.h

type AvbFooter struct {
	Magic             [4]byte
	VersionMajor      uint32
	VersionMinor      uint32
	OriginalImageSize uint64
	VBMetaOffset      uint64
	VBMetaSize        uint64
	Reserved          [28]byte
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_vbmeta_image.h

type AvbVBMetaImageHeader struct {
	Magic                       [4]byte
	RequiredLibavbVersionMajor  uint32
	RequiredLibavbVersionMinor  uint32
	AuthenticationDataBlockSize uint64
	AuxiliaryDataBlockSize      uint64
	AlgorithmType               uint32
	HashOffset                  uint64
	HashSize                    uint64
	SignatureOffset             uint64
	SignatureSize               uint64
	PublicKeyOffset             uint64
	PublicKeySize               uint64
	PublicKeyMetadataOffset     uint64
	PublicKeyMetadataSize       uint64
	DescriptorsOffset           uint64
	DescriptorsSize             uint64
	RollbackIndex               uint64
	Flags                       uint32
	RollbackIndexLocation       uint32
	ReleaseString               [48]byte
	Reserved                    [80]byte
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_descriptor.h

type AvbDescriptorHeader struct {
	Tag               uint64
	NumBytesFollowing uint64
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_property_descriptor.h

type AvbPropertyDescriptor struct {
	ParentDescriptor AvbDescriptorHeader
	KeyNumBytes      uint64
	ValueNumBytes    uint64
}

type MagicProvider interface {
	GetMagic() [4]byte
}

func (h LocalFileHeader) GetMagic() [4]byte      { return h.Magic }
func (h CdFileHeader) GetMagic() [4]byte         { return h.Magic }
func (r EocdRecord) GetMagic() [4]byte           { return r.Magic }
func (l Zip64EocdLocator) GetMagic() [4]byte     { return l.Magic }
func (r Zip64EocdRecord) GetMagic() [4]byte      { return r.Magic }
func (s ErofsSuperblock) GetMagic() [4]byte      { return s.Magic }
func (s SparseStub) GetMagic() [4]byte           { return s.Magic }
func (s ErofsStub) GetMagic() [4]byte            { return s.Magic }
func (a AvbFooter) GetMagic() [4]byte            { return a.Magic }
func (a AvbVBMetaImageHeader) GetMagic() [4]byte { return a.Magic }

type Image interface {
	io.ReaderAt
	io.ReadCloser
	OpenStream(offset, end uint64) (io.ReadCloser, error)
}

type RemoteImage struct {
	url    string
	client *http.Client
	offset int64
}

func (r *RemoteImage) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	req, _ := http.NewRequest("GET", r.url, nil)
	end := off + int64(len(p)) - 1
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, end))

	res, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}

	return io.ReadFull(res.Body, p)
}

func (r *RemoteImage) Read(p []byte) (int, error) {
	n, err := r.ReadAt(p, r.offset)
	r.offset += int64(n)
	return n, err
}

func (r *RemoteImage) Close() error {
	return nil
}

func (r *RemoteImage) OpenStream(offset uint64, end uint64) (io.ReadCloser, error) {
	req, _ := http.NewRequest("GET", r.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))

	res, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed: %w", err)
	}
	if res.StatusCode != http.StatusPartialContent {
		//goland:noinspection GoUnhandledErrorResult
		res.Body.Close()
		return nil, fmt.Errorf("GET request rejected with status: %s", res.Status)
	}

	return res.Body, nil
}

type LocalImage struct {
	file *os.File
}

func (l *LocalImage) ReadAt(p []byte, off int64) (int, error) {
	return l.file.ReadAt(p, off)
}

func (l *LocalImage) Read(p []byte) (int, error) {
	return l.file.Read(p)
}

func (l *LocalImage) Close() error {
	return l.file.Close()
}

func (l *LocalImage) OpenStream(offset, end uint64) (io.ReadCloser, error) {
	length := int64(end - offset + 1)
	return io.NopCloser(io.NewSectionReader(l.file, int64(offset), length)), nil
}

const (
	ChunkSize = 16384

	SizeofErofsInodeCompact     = 32
	SizeofErofsInodeExtended    = 64
	SizeofErofsXattrIbodyHeader = 12
	SizeofSharedXattrs          = 4
	SizeofErofsXattrEntry       = 4

	// https://erofs.docs.kernel.org/en/latest/ondisk/core_ondisk.html

	ErofsFtRegFile = 1
	ErofsFtDir     = 2
	ErofsFtSymlink = 7

	// https://android.googlesource.com/kernel/common/+/refs/heads/android17-6.18-2026-09/fs/erofs/erofs_fs.h

	ErofsFeatureIncompatDeviceTable = uint64(0x00000008)
	ErofsInodeFlatInline            = 2
	ErofsInodeChunkBased            = 4
	ErofsIVersionMask               = uint16(0x01)
	ErofsIDatalayoutMask            = uint16(0x07)
	ErofsIVersionBit                = 0
	ErofsIDatalayoutBit             = 1
	ErofsChunkFormatIndexes         = 0x0020
	ErofsInodeLayoutCompact         = 0
	ErofsInodeLayoutExtended        = 1

	// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_vbmeta_image.h
	// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_footer.h
	// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_descriptor.h

	AvbMagic                 = "AVB0"
	AvbVBMetaImageHeaderSize = 256
	AvbFooterMagic           = "AVBf"
	AvbFooterSize            = 64
	AvbDescriptorTagProperty = 0
)

var emptyMagic [4]byte

var Version = "development"

//goland:noinspection GoUnhandledErrorResult
func fetchRange(image Image, offset uint64, end uint64) ([]byte, error) {
	size := int(end - offset + 1)
	buf := make([]byte, size)
	n, err := image.ReadAt(buf, int64(offset))
	if err != nil && (!errors.Is(err, io.EOF) || n != size) {
		return nil, err
	}
	return buf, nil
}

func fetchStruct(image Image, offset uint64, target any, label string, magic [4]byte, order binary.ByteOrder) error {
	sizeofStruct := binary.Size(target)
	end := offset + uint64(sizeofStruct) - 1

	buf, err := fetchRange(image, offset, end)
	if err != nil {
		return err
	}

	return readStruct(buf, 0, target, label, magic, order)
}

func readStruct(buf []byte, offset uint64, target any, label string, magic [4]byte, order binary.ByteOrder) error {
	reader := bytes.NewReader(buf[offset:])
	if err := binary.Read(reader, order, target); err != nil {
		return fmt.Errorf("%s: failed to parse data: %w", label, err)
	}

	if magic != emptyMagic {
		if provider, ok := target.(MagicProvider); ok {
			if magic != provider.GetMagic() {
				return fmt.Errorf("%s: unexpected magic value: %v", label, provider.GetMagic())
			}
		} else {
			return fmt.Errorf("%s: failed to apply magic provider", label)
		}
	}

	return nil
}

//goland:noinspection GoUnhandledErrorResult
func fetchFileZip(image Image, offset uint64, end uint64, uncompressedSize uint64, partitionFilename string, targetFilename string, compressionMethod uint16) (uint32, error) {
	stream, err := image.OpenStream(offset, end)
	if err != nil {
		return 0, err
	}
	defer stream.Close()

	fmt.Print("partition image: streaming file ...")

	if targetFilename == "" {
		targetFilename = partitionFilename
	}
	out, err := os.Create(targetFilename)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("partition image: failed to create local partition image: %v", err)
	}
	defer out.Close()

	var dataReader io.Reader = stream
	if compressionMethod == 8 {
		flateReader := flate.NewReader(stream)
		defer flateReader.Close()
		dataReader = flateReader
	}

	table := crc32.MakeTable(crc32.IEEE)
	hash := crc32.New(table)

	chunk := make([]byte, ChunkSize)
	var totalWritten int64 = 0

	for {
		bytesRead, readErr := dataReader.Read(chunk)
		if bytesRead > 0 {
			bytesWritten, writeErr := out.Write(chunk[:bytesRead])
			if writeErr != nil {
				fmt.Println()
				return 0, fmt.Errorf("partition image: failed to write data to buffer: %v", writeErr)
			}
			hash.Write(chunk[:bytesRead])
			totalWritten += int64(bytesWritten)
			fmt.Printf("\r\033[2Kpartition image: read %d of %d bytes", totalWritten, uncompressedSize)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fmt.Println()
			return 0, fmt.Errorf("partition image: block read error: %v", readErr)
		}
	}

	fmt.Println("\r\033[2Kpartition image: successfully written")

	return hash.Sum32(), nil
}

func fetchFileErofs(image Image, offset uint64, nid uint64, filePath []string, depth int, superblock ErofsSuperblock) error {
	dirents, err := fetchInodeDirents(nid, image, offset, superblock)
	if err != nil {
		return err
	}

	//goland:noinspection ALL
	if dirent, ok := dirents[filePath[depth]]; ok {
		if depth < len(filePath)-1 {
			if dirent.FileType == ErofsFtSymlink {
				return fmt.Errorf("erofs: symlinks are not currently supported, contact developer")
			} else if dirent.FileType != ErofsFtDir {
				return fmt.Errorf("erofs: expected dir but got unexpected type: %d", dirent.FileType)
			}
			return fetchFileErofs(image, offset, dirent.Nid, filePath, depth+1, superblock)
		} else {
			if dirent.FileType != ErofsFtRegFile {
				return fmt.Errorf("erofs: expected file but got unexpected type: %d", dirent.FileType)
			}
			blockSize := uint64(1) << superblock.BlkSizeBits
			nidOffset := uint64(superblock.MetaBlkAddr)*blockSize + dirent.Nid*SizeofErofsInodeCompact

			sizeofInode, sizeofXattr, fileSize, err := fetchInode(image, offset+nidOffset, ErofsInodeChunkBased)

			var chunkIndex ErofsInodeChunkIndex
			sizeofChunkIndex := uint64(binary.Size(chunkIndex))

			chunkIndexOffset := offset + nidOffset + sizeofInode + sizeofXattr
			totalBlocks := (fileSize + blockSize - 1) / blockSize
			buf, err := fetchRange(image, chunkIndexOffset, chunkIndexOffset+totalBlocks*sizeofChunkIndex-1)
			if err != nil {
				return err
			}

			var startBlk uint32
			for i := range totalBlocks {
				chunkIndexOffset = sizeofChunkIndex * i
				if err = readStruct(buf, chunkIndexOffset, &chunkIndex, "chunk index", emptyMagic, binary.LittleEndian); err != nil {
					return err
				}
				if i == 0 {
					startBlk = chunkIndex.StartBlk
				} else {
					if chunkIndex.StartBlk != startBlk+uint32(i) {
						return fmt.Errorf("erofs: file blocks are non-contiguous, expected %d but got %d", startBlk+uint32(i), chunkIndex.StartBlk)
					}
				}
			}

			fileOffset := offset + uint64(startBlk)*blockSize
			stream, err := image.OpenStream(fileOffset, fileOffset+fileSize-1)
			if err != nil {
				return err
			}
			defer stream.Close()

			fmt.Print("file path: streaming file ...")

			out, err := os.Create(filePath[depth])
			if err != nil {
				fmt.Println()
				return fmt.Errorf("file path: failed to create local file path: %v", err)
			}
			defer out.Close()

			chunk := make([]byte, ChunkSize)
			var totalWritten int64 = 0

			for {
				bytesRead, readErr := stream.Read(chunk)
				if bytesRead > 0 {
					bytesWritten, writeErr := out.Write(chunk[:bytesRead])
					if writeErr != nil {
						fmt.Println()
						return fmt.Errorf("file path: failed to write data to buffer: %v", writeErr)
					}
					totalWritten += int64(bytesWritten)
					fmt.Printf("\r\033[2Kfile path: read %d of %d bytes", totalWritten, fileSize)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					fmt.Println()
					return fmt.Errorf("file path: block read error: %v", readErr)
				}
			}

			fmt.Println("\r\033[2Kfile path: successfully written")

			return nil
		}
	} else {
		return fmt.Errorf("file path: failed to find /%s", strings.Join(filePath[:depth+1], "/"))
	}
}

func fetchInode(image Image, offset uint64, expectedDataLayout uint16) (uint64, uint64, uint64, error) {
	inodeEnd := offset + SizeofErofsInodeExtended + SizeofErofsXattrIbodyHeader + SizeofSharedXattrs - 1
	buf, err := fetchRange(image, offset, inodeEnd)
	if err != nil {
		// TODO: Allow io.EOF
		return 0, 0, 0, err
	}

	var inodeCompact ErofsInodeCompact
	if err = readStruct(buf, 0, &inodeCompact, "inode compact", emptyMagic, binary.LittleEndian); err != nil {
		return 0, 0, 0, err
	}

	inodeVersion := (inodeCompact.Format >> ErofsIVersionBit) & ErofsIVersionMask
	datalayout := (inodeCompact.Format >> ErofsIDatalayoutBit) & ErofsIDatalayoutMask

	if datalayout != expectedDataLayout {
		return 0, 0, 0, fmt.Errorf("erofs: inode format is not currently supported: %d", datalayout)
	}
	if expectedDataLayout == ErofsInodeChunkBased {
		indexes := inodeCompact.ChunkInfo.Format & ErofsChunkFormatIndexes
		if indexes != 0 {
			return 0, 0, 0, fmt.Errorf("erofs: indexed chunk entries are not currently supported")
		}
	}
	if inodeCompact.XattrCount > 3 {
		return 0, 0, 0, fmt.Errorf("erofs: unexpected XattrCount: %d", inodeCompact.XattrCount)
	}

	var sizeofInode uint64
	var targetSize uint64
	if inodeVersion == ErofsInodeLayoutCompact {
		sizeofInode = SizeofErofsInodeCompact
		targetSize = uint64(inodeCompact.Size)
	} else if inodeVersion == ErofsInodeLayoutExtended {
		sizeofInode = SizeofErofsInodeExtended
		var inodeExtended ErofsInodeExtended
		if err = readStruct(buf, 0, &inodeExtended, "inode extended", emptyMagic, binary.LittleEndian); err != nil {
			return 0, 0, 0, err
		}
		targetSize = inodeExtended.Size
	}

	var xattrIbody ErofsXattrIbodyHeader
	if err = readStruct(buf, sizeofInode, &xattrIbody, "xattr ibody", emptyMagic, binary.LittleEndian); err != nil {
		return 0, 0, 0, err
	}

	if xattrIbody.SharedCount > 1 {
		return 0, 0, 0, fmt.Errorf("erofs: unexpected SharedCount: %d", xattrIbody.SharedCount)
	}

	sizeofXattr := uint64(binary.Size(xattrIbody)) + SizeofErofsXattrEntry

	return sizeofInode, sizeofXattr, targetSize, nil
}

//goland:noinspection GoUnhandledErrorResult
func fetchInodeDirents(nid uint64, image Image, offset uint64, superblock ErofsSuperblock) (map[string]ErofsDirent, error) {
	blockSize := uint64(1) << superblock.BlkSizeBits

	nidOffset := uint64(superblock.MetaBlkAddr)*blockSize + nid*SizeofErofsInodeCompact

	sizeofInode, sizeofXattr, direntSize, err := fetchInode(image, offset+nidOffset, ErofsInodeFlatInline)
	if err != nil {
		return nil, err
	}

	if direntSize > blockSize {
		return nil, fmt.Errorf("erofs: multi-block dirents are not currently supported")
	}

	direntOffset := offset + nidOffset + sizeofInode + sizeofXattr
	buf, err := fetchRange(image, direntOffset, direntOffset+direntSize-1)
	if err != nil {
		return nil, err
	}

	fileMap := make(map[string]ErofsDirent)

	var dirent ErofsDirent
	err = readStruct(buf, 0, &dirent, "dirent", emptyMagic, binary.LittleEndian)
	if err != nil {
		return nil, fmt.Errorf("dirents: failed to read initial dirent: %w", err)
	}

	numEntries := int(dirent.NameOff) / 12
	if numEntries == 0 {
		return fileMap, nil
	}

	dirents := make([]ErofsDirent, numEntries)
	for i := range numEntries {
		err = readStruct(buf, uint64(i*12), &dirent, "dirent", emptyMagic, binary.LittleEndian)
		if err != nil {
			return nil, fmt.Errorf("dirents: failed to read dirent at index %d: %w", i, err)
		}
		dirents[i] = dirent
	}

	for i := range numEntries {
		start := dirents[i].NameOff
		var end uint16

		if i+1 < numEntries {
			end = dirents[i+1].NameOff
		} else {
			end = uint16(len(buf))
		}

		nameBytes := buf[start:end]
		parts := bytes.Split(nameBytes, []byte{0x00})
		fileName := string(parts[0])
		fileMap[fileName] = dirents[i]
	}

	return fileMap, nil
}

func printAvbPropertyDescriptors(buf []byte, avbHeader AvbVBMetaImageHeader) error {
	var avbDescriptor AvbDescriptorHeader
	var avbPropertyDescriptor AvbPropertyDescriptor
	sizeofAvbDescriptor := binary.Size(avbDescriptor)
	sizeofAvbPropertyDescriptor := uint64(binary.Size(avbPropertyDescriptor))

	var offset = AvbVBMetaImageHeaderSize + avbHeader.AuthenticationDataBlockSize + avbHeader.DescriptorsOffset
	var descriptorsEnd = offset + avbHeader.DescriptorsSize
	for offset < descriptorsEnd {
		err := readStruct(buf, offset, &avbDescriptor, "avb descriptor", emptyMagic, binary.BigEndian)
		if err != nil {
			return fmt.Errorf("file path: failed to read avb descriptor: %w", err)
		}

		if avbDescriptor.Tag == AvbDescriptorTagProperty {
			err = readStruct(buf, offset, &avbPropertyDescriptor, "avb property descriptor", emptyMagic, binary.BigEndian)
			if err != nil {
				return fmt.Errorf("file path: failed to read avb property descriptor: %w", err)
			}
			keyOffset := offset + sizeofAvbPropertyDescriptor
			keyEnd := keyOffset + avbPropertyDescriptor.KeyNumBytes
			valueOffset := keyEnd + 1
			valueEnd := valueOffset + avbPropertyDescriptor.ValueNumBytes
			key := string(buf[keyOffset:keyEnd])
			value := string(buf[valueOffset:valueEnd])
			fmt.Printf("%s=%s\n", key, value)
		}

		offset += uint64(sizeofAvbDescriptor) + avbDescriptor.NumBytesFollowing
	}
	return nil
}

func findAndPrintAvbPropertyDescriptors(image Image, offset uint64, size uint64) error {
	var avbHeader AvbVBMetaImageHeader
	var avbHeaderMagic [4]byte
	copy(avbHeaderMagic[:], AvbMagic)
	err := fetchStruct(image, offset, &avbHeader, "avb header", avbHeaderMagic, binary.BigEndian)
	if err != nil {
		var avbFooter AvbFooter
		var avbFooterMagic [4]byte
		copy(avbFooterMagic[:], AvbFooterMagic)
		footerOffset := offset + size - AvbFooterSize
		err = fetchStruct(image, footerOffset, &avbFooter, "avb footer", avbFooterMagic, binary.BigEndian)
		if err != nil {
			return err
		}

		offset += avbFooter.VBMetaOffset

		err = fetchStruct(image, offset, &avbHeader, "avb header", avbHeaderMagic, binary.BigEndian)
		if err != nil {
			return err
		}
	}

	avbBuf, err := fetchRange(image, offset, offset+AvbVBMetaImageHeaderSize+avbHeader.AuthenticationDataBlockSize+avbHeader.AuxiliaryDataBlockSize-1)
	if err != nil {
		return err
	}
	return printAvbPropertyDescriptors(avbBuf, avbHeader)

}

//goland:noinspection GoUnhandledErrorResult
func findAndReadCentralDirectory(image Image, partitionFilename string, filePath string, list bool, avb bool, sofOffset uint64, eofOffset uint64) error {
	var eocdHeader EocdRecord
	var eocd64locator Zip64EocdLocator
	var eocd64record Zip64EocdRecord

	sizeofEocdHeader := binary.Size(eocdHeader)
	sizeofEocd64locator := binary.Size(eocd64locator)
	sizeofEocd64record := binary.Size(eocd64record)

	eocdOffset := eofOffset - uint64(sizeofEocdHeader+sizeofEocd64locator+sizeofEocd64record)
	eocdBuf, err := fetchRange(image, eocdOffset, eofOffset-1)
	if err != nil {
		return err
	}

	eocdIdx := uint64(sizeofEocd64record + sizeofEocd64locator)
	err = readStruct(eocdBuf, eocdIdx, &eocdHeader, "end of central directory", [4]byte{'P', 'K', 0x05, 0x06}, binary.LittleEndian)
	if err != nil {
		if eocdOffset < 3998 {
			return fmt.Errorf("end of central directory: file smaller than fallback offset: %w", err)
		}
		eocdOffset -= 3998
		eocdBuf, err = fetchRange(image, eocdOffset, eofOffset-1)
		// TODO: Why did I previously allow this error? Is it still needed?
		if err != nil {
			return err
		}
		eocdIdx2 := bytes.LastIndex(eocdBuf, []byte{'P', 'K', 0x05, 0x06})
		if eocdIdx2 == -1 {
			return fmt.Errorf("end of central directory: unable to find magic")
		}
		err = readStruct(eocdBuf, uint64(eocdIdx2), &eocdHeader, "end of central directory", [4]byte{'P', 'K', 0x05, 0x06}, binary.LittleEndian)
		if err != nil {
			return err
		}
	}

	cdOffset := uint64(eocdHeader.CdOffset)
	cdSize := uint64(eocdHeader.CdSize)

	if eocdHeader.CdOffset == 0xFFFFFFFF {
		eocd64recordIdx := eocdIdx - uint64(sizeofEocd64record+sizeofEocd64locator)
		err = readStruct(eocdBuf, eocd64recordIdx, &eocd64record, "zip64 end of central directory record", [4]byte{'P', 'K', 0x06, 0x06}, binary.LittleEndian)
		if err != nil {
			return err
		}

		cdOffset = eocd64record.CdOffset
		cdSize = eocd64record.CdSize
	}

	innerCdOffset := sofOffset + cdOffset
	cdBuf, err := fetchRange(image, innerCdOffset, innerCdOffset+cdSize-1)
	if err != nil {
		return err
	}

	var offset uint64 = 0
	var cdfHeader CdFileHeader
	var lfHeader LocalFileHeader

	var nestedZipRegex = regexp.MustCompile(`.+/image-.+\.zip`)
	var partitionRegex = regexp.MustCompile(fmt.Sprintf(`(^|/)%s`, regexp.QuoteMeta(partitionFilename)))

	sizeofCdfHeader := uint64(binary.Size(cdfHeader))
	sizeofLfHeader := uint64(binary.Size(lfHeader))

	for offset < cdSize {
		if offset+sizeofCdfHeader > cdSize {
			return fmt.Errorf("central directory: unexpected end of buffer")
		}

		err = readStruct(cdBuf, offset, &cdfHeader, "central directory file header", [4]byte{'P', 'K', 0x01, 0x02}, binary.LittleEndian)
		if err != nil {
			return err
		}

		n := uint64(cdfHeader.FilenameLen)
		m := uint64(cdfHeader.ExtraFieldLen)
		k := uint64(cdfHeader.FileCommentLen)

		filename := string(cdBuf[offset+sizeofCdfHeader : offset+sizeofCdfHeader+n])

		var uncompressedSize = uint64(cdfHeader.UncompressedSize)
		var compressedSize = uint64(cdfHeader.CompressedSize)
		var lfhOffset = uint64(cdfHeader.LfhOffset)

		if cdfHeader.LfhOffset == 0xFFFFFFFF || cdfHeader.CompressedSize == 0xFFFFFFFF || cdfHeader.UncompressedSize == 0xFFFFFFFF {
			extraBuf := cdBuf[offset+sizeofCdfHeader+n : offset+sizeofCdfHeader+n+m]
			extraReader := bytes.NewReader(extraBuf)

			var uncompressedSize64 uint64
			var compressedSize64 uint64
			var lfhOffset64 uint64

			for extraReader.Len() >= 4 {
				var headerID uint16
				var dataSize uint16

				binary.Read(extraReader, binary.LittleEndian, &headerID)
				binary.Read(extraReader, binary.LittleEndian, &dataSize)

				if headerID == 0x0001 {
					if cdfHeader.UncompressedSize == 0xFFFFFFFF {
						binary.Read(extraReader, binary.LittleEndian, &uncompressedSize64)
						uncompressedSize = uncompressedSize64
					}
					if cdfHeader.CompressedSize == 0xFFFFFFFF {
						binary.Read(extraReader, binary.LittleEndian, &compressedSize64)
						compressedSize = compressedSize64
					}
					if cdfHeader.LfhOffset == 0xFFFFFFFF {
						binary.Read(extraReader, binary.LittleEndian, &lfhOffset64)
						lfhOffset = lfhOffset64
					}
					break
				} else {
					if _, err := extraReader.Seek(int64(dataSize), io.SeekCurrent); err != nil {
						return fmt.Errorf("extra: failed to seek: %w", err)
					}
				}
			}
		}

		if list {
			var compressedSizeHuman string
			if compressedSize > 1024*1024*1024 {
				compressedSizeHuman = fmt.Sprintf(" (%0.3f gb)", float32(compressedSize)/1024/1024/1024)
			} else if compressedSize > 1024*1024 {
				compressedSizeHuman = fmt.Sprintf(" (%0.3f mb)", float32(compressedSize)/1024/1024)
			} else if compressedSize > 1024 {
				compressedSizeHuman = fmt.Sprintf(" (%0.3f kb)", float32(compressedSize)/1024)
			} else {
				compressedSizeHuman = ""
			}
			prefix := ""
			if sofOffset != 0 {
				prefix = "- "
			}
			fmt.Printf("%s%s | Offset: %d | Size: %d%s | Compression: %d\n", prefix, filename, lfhOffset, compressedSize, compressedSizeHuman, cdfHeader.CompressionMethod)
		}
		if nestedZipRegex.MatchString(filename) || (partitionFilename != "" && partitionRegex.MatchString(filename)) {
			err = fetchStruct(image, sofOffset+lfhOffset, &lfHeader, "local file header", [4]byte{'P', 'K', 0x03, 0x04}, binary.LittleEndian)
			if err != nil {
				return err
			}

			n := uint64(lfHeader.FilenameLen)
			m := uint64(lfHeader.ExtraFieldLen)

			lfOffset := sofOffset + lfhOffset + sizeofLfHeader + n + m

			//goland:noinspection GoRedundantElseInIf
			if nestedZipRegex.MatchString(filename) {
				if err = findAndReadCentralDirectory(image, partitionFilename, filePath, list, avb, lfOffset, lfOffset+compressedSize); err != nil {
					return err
				} else if !list {
					return nil
				}
			} else {
				//goland:noinspection GoRedundantElseInIf
				if filePath != "" {
					if cdfHeader.CompressionMethod == 8 {
						return fmt.Errorf("partition: extracting files in compressed images is not currently supported")
					}
					return findAndExtractFile(image, filePath, lfOffset)
				} else {
					if cdfHeader.CompressionMethod == 8 || !avb {
						// allows defer in a loop
						return func() error {
							var targetFilename string
							if avb {
								tmpFile, err := os.CreateTemp("", "pixel-plucker-*.img")
								if err != nil {
									tmpFile.Close()
									os.Remove(tmpFile.Name())
									return fmt.Errorf("central directory: failed to create temp file: %w", err)
								}
								defer os.Remove(tmpFile.Name())
								targetFilename = tmpFile.Name()
								tmpFile.Close()
							}

							hash, err := fetchFileZip(image, lfOffset, lfOffset+compressedSize-1, uncompressedSize, partitionFilename, targetFilename, cdfHeader.CompressionMethod)
							if err != nil {
								return err
							}
							if hash != cdfHeader.CRC32 {
								os.Remove(partitionFilename)
								return fmt.Errorf("central directory: hash mismatch")
							} else if avb {
								file, err := os.Open(targetFilename)
								if err != nil {
									return fmt.Errorf("failed to open temporary file: %w", err)
								}
								defer file.Close()
								avbImage := &LocalImage{file: file}
								return findAndPrintAvbPropertyDescriptors(avbImage, 0, uncompressedSize)
							}

							return nil
						}()
					} else {
						return findAndPrintAvbPropertyDescriptors(image, lfOffset, uncompressedSize)
					}
				}
			}
		}

		offset += sizeofCdfHeader + n + m + k
	}

	if !list {
		//goland:noinspection GoRedundantElseInIf
		if sofOffset == 0 {
			return fmt.Errorf("failed to find nested image zip")
		} else {
			return fmt.Errorf("failed to find partition filename in nested image zip")
		}
	}

	return nil
}

func findAndExtractFile(image Image, filePath string, offset uint64) error {
	if !path.IsAbs(filePath) {
		return fmt.Errorf("partition: filePath must be an absolute path")
	}

	var sparseStub SparseStub
	var sparseMagic [4]byte
	binary.LittleEndian.PutUint32(sparseMagic[:], 0xed26ff3a)
	err := fetchStruct(image, offset, &sparseStub, "sparse stub", sparseMagic, binary.LittleEndian)
	if err == nil {
		return fmt.Errorf("partition: sparse image format is not currently supported")
	}

	var erofsStub ErofsStub
	var erofsMagic [4]byte
	binary.LittleEndian.PutUint32(erofsMagic[:], 0xe0f5e1e2)
	erofsOffset := offset + 0x400
	err = fetchStruct(image, erofsOffset, &erofsStub, "erofs stub", erofsMagic, binary.LittleEndian)
	//goland:noinspection GoRedundantElseInIf
	if err == nil {
		var superblock ErofsSuperblock
		err = fetchStruct(image, erofsOffset, &superblock, "erofs superblock", erofsMagic, binary.LittleEndian)
		if err != nil {
			return err
		}

		deviceTable := uint64(superblock.FeatureIncompat) & ErofsFeatureIncompatDeviceTable
		if deviceTable != 0 {
			return fmt.Errorf("erofs: superblock feature is not currently supported: %d", deviceTable)
		}

		var filePathParts []string
		for {
			base := filepath.Base(filePath)
			if base == string(filepath.Separator) {
				break
			}
			filePathParts = append([]string{base}, filePathParts...)
			filePath = filepath.Dir(filePath)
		}

		return fetchFileErofs(image, offset, uint64(superblock.RootNid), filePathParts, 0, superblock)
	} else {
		ext4Offset := offset + 0x400 + 56
		buf, err := fetchRange(image, ext4Offset, ext4Offset+2-1)
		if err != nil {
			return err
		}
		if binary.LittleEndian.Uint16(buf) == 0xef53 {
			return fmt.Errorf("partition: ext4 format is not currently supported")
		} else {
			return fmt.Errorf("partition: format is unknown")
		}
	}
}

func hasImgFlag() bool {
	for _, arg := range os.Args {
		if arg == "-i" || arg == "--img" {
			return true
		}
	}
	return false
}

//goland:noinspection GoUnhandledErrorResult
func main() {
	var version bool
	flag.BoolVar(&version, "v", false, "")
	flag.BoolVar(&version, "version", false, "")

	var list bool
	flag.BoolVar(&list, "l", false, "")
	flag.BoolVar(&list, "list", false, "")

	var avb bool
	flag.BoolVar(&avb, "a", false, "")
	flag.BoolVar(&avb, "avb", false, "")

	var img bool
	flag.BoolVar(&img, "i", false, "")
	flag.BoolVar(&img, "img", false, "")

	flag.Usage = func() {
		if !hasImgFlag() {
			fmt.Fprintf(os.Stderr, "Usage: pluck [flags] <factoryImageUrlOrFile> [partitionFilename [filePath]]\n\n")
			fmt.Fprintln(os.Stderr, "Arguments:")
			fmt.Fprintln(os.Stderr, "  factoryImageUrlOrFile   Remote URL or local file path of the factory image")
			fmt.Fprintln(os.Stderr, "  partitionFilename       Name of the partition to download (optional)")
			fmt.Fprintln(os.Stderr, "  filePath                Path to file in partition to download (optional)")
			fmt.Fprintln(os.Stderr, "\nFlags:")
			fmt.Fprintln(os.Stderr, "  -v, --version           Print version and exit")
			fmt.Fprintln(os.Stderr, "  -l, --list              List filenames")
			fmt.Fprintln(os.Stderr, "  -a, --avb               List AVB props")
			fmt.Fprintln(os.Stderr, "  -i, --img               Work with partition images directly")
		} else {
			fmt.Fprintf(os.Stderr, "Usage: pluck --img [-a|--avb] <partitionImageUrlOrFile> [filePath]\n\n")
			fmt.Fprintln(os.Stderr, "Arguments:")
			fmt.Fprintln(os.Stderr, "  partitionImageUrlOrFile URL or local file path of the partition image")
			fmt.Fprintln(os.Stderr, "  filePath                Path to file in partition to download (optional)")
			fmt.Fprintln(os.Stderr, "\nFlags:")
			fmt.Fprintln(os.Stderr, "  -a, --avb               List AVB props")
		}

	}

	flag.Parse()
	args := flag.Args()

	if version {
		fmt.Fprintf(os.Stderr, "pluck %s\n", Version)
		os.Exit(0)
	}

	if list && avb {
		fmt.Fprintln(os.Stderr, "Error: list and avb are not compatible")
		flag.Usage()
		os.Exit(1)
	}
	if len(args) < 1 || len(args) > 3 {
		flag.Usage()
		os.Exit(1)
	}
	if !img {
		if !list && len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: partitionFilename is required when list flag is not provided")
			flag.Usage()
			os.Exit(1)
		}
		if avb && len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: partitionFilename is required when avb flag is provided")
			flag.Usage()
			os.Exit(1)
		}
		if avb && len(args) == 3 {
			fmt.Fprintln(os.Stderr, "Error: filePath and avb are not compatible")
			flag.Usage()
			os.Exit(1)
		}
		if list && len(args) > 1 {
			fmt.Fprintln(os.Stderr, "Error: listing of files in partitionFilename is not supported")
			flag.Usage()
			os.Exit(1)
		}
	} else {
		if list {
			fmt.Fprintln(os.Stderr, "Error: list and img are not compatible")
			flag.Usage()
			os.Exit(1)
		}
		if len(args) < 1 || len(args) > 2 {
			flag.Usage()
			os.Exit(1)
		}
		if avb && len(args) > 1 {
			fmt.Fprintln(os.Stderr, "Error: avb is only compatible with top level partition images when img flag is provided")
			flag.Usage()
			os.Exit(1)
		}
		if !avb && len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: filePath is required when avb flag is not provided")
			flag.Usage()
			os.Exit(1)
		}
	}

	urlOrFile := args[0]
	partitionFilename := ""
	filePath := ""
	if !img {
		if len(args) > 1 {
			partitionFilename = args[1]
		}
		if len(args) > 2 {
			filePath = args[2]
		}
	} else {
		if len(args) > 1 {
			filePath = args[1]
		}
	}

	var image Image
	var imageSize int64
	//goland:noinspection HttpUrlsUsage
	if strings.HasPrefix(urlOrFile, "http://") || strings.HasPrefix(urlOrFile, "https://") {
		client := &http.Client{}
		res, err := client.Head(urlOrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "factory image: HEAD request failed: %v\n", err)
			os.Exit(1)
		}
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "factory image: HEAD request rejected with status: %s\n", res.Status)
			os.Exit(1)
		}

		image = &RemoteImage{
			url:    urlOrFile,
			client: client,
		}
		contentLengthStr := res.Header.Get("Content-Length")
		imageSize, err = strconv.ParseInt(contentLengthStr, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "factory image: failed to parse Content-Length: %v\n", err)
			os.Exit(1)
		}

	} else {
		file, err := os.Open(urlOrFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open local file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			file.Close()
			fmt.Fprintf(os.Stderr, "failed to stat local file: %v\n", err)
			os.Exit(1)
		}

		image = &LocalImage{file: file}
		imageSize = info.Size()
	}

	var err error
	if !img {
		err = findAndReadCentralDirectory(image, partitionFilename, filePath, list, avb, 0, uint64(imageSize))
	} else if !avb {
		err = findAndExtractFile(image, filePath, 0)
	} else {
		err = findAndPrintAvbPropertyDescriptors(image, 0, uint64(imageSize))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
