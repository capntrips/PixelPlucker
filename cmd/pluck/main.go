package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
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
	Crc32             uint32
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
	Crc32             uint32
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

type AvbDescriptor struct {
	Tag               uint64
	NumBytesFollowing uint64
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_property_descriptor.h

type AvbPropertyDescriptor struct {
	ParentDescriptor AvbDescriptor
	KeyNumBytes      uint64
	ValueNumBytes    uint64
}

// https://android.googlesource.com/platform/external/avb/+/refs/tags/android-17.0.0_r1/libavb/avb_hash_descriptor.h

type AvbHashDescriptor struct {
	ParentDescriptor AvbDescriptor
	ImageSize        uint64
	HashAlgorithm    [32]byte
	PartitionNameLen uint32
	SaltLen          uint32
	DigestLen        uint32
	Flags            uint32
	Reserved         [60]byte
}

// https://source.android.com/docs/core/architecture/bootloader/tools/pixel/fw_unpack/fbpacktool.py

type FastBootPackHeader struct {
	Magic           [4]byte // FBPK
	Version         uint32
	HeaderSize      uint32
	EntryHeaderSize uint32
	Platform        [16]byte
	PackVersion     [64]byte
	SlotType        uint32
	DataAlign       uint32
	TotalEntries    uint32
	TotalSize       uint32
}

type FastBootPackEntry struct {
	Type    uint32
	Name    [36]byte
	Product [40]byte
	Offset  uint64
	Size    uint64
	Slotted uint32
	Crc32   uint32
}

type BootloaderDescriptor struct {
	Magic          [4]byte // FBPK
	Unknown        uint32
	Ar             uint32
	PayloadSize    uint32
	DescriptorSize uint32
	Reserved       [12]byte
	Sha256         [32]byte
}

//goland:noinspection GoUnusedExportedType
type BootloaderDescriptorG3 struct {
	Magic          [4]byte // FBPK
	Unknown        uint32
	Ar             uint32
	PayloadSize    uint32
	DescriptorSize uint32
	Reserved       [12]byte
	Sha512         [64]byte
}

type BootloaderDescriptorG5 struct {
	Magic       [4]byte // FBPK
	Ar          uint32
	Unknown     uint32
	PayloadSize uint32
	Reserved    [16]byte
	Sha384      [32]byte
}

type MagicProvider interface {
	GetMagic() [4]byte
}

func (h LocalFileHeader) GetMagic() [4]byte        { return h.Magic }
func (h CdFileHeader) GetMagic() [4]byte           { return h.Magic }
func (r EocdRecord) GetMagic() [4]byte             { return r.Magic }
func (l Zip64EocdLocator) GetMagic() [4]byte       { return l.Magic }
func (r Zip64EocdRecord) GetMagic() [4]byte        { return r.Magic }
func (s ErofsSuperblock) GetMagic() [4]byte        { return s.Magic }
func (s SparseStub) GetMagic() [4]byte             { return s.Magic }
func (s ErofsStub) GetMagic() [4]byte              { return s.Magic }
func (f AvbFooter) GetMagic() [4]byte              { return f.Magic }
func (h AvbVBMetaImageHeader) GetMagic() [4]byte   { return h.Magic }
func (h FastBootPackHeader) GetMagic() [4]byte     { return h.Magic }
func (d BootloaderDescriptor) GetMagic() [4]byte   { return d.Magic }
func (d BootloaderDescriptorG5) GetMagic() [4]byte { return d.Magic }

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
	AvbDescriptorTagHash     = 2

	// https://source.android.com/static/docs/core/architecture/bootloader/tools/pixel/fw_unpack/fbpack.py

	FbpackMagic          = "FBPK"
	FbpackVersion        = 2
	FbpackPartitionTable = 0
	FbpackPartitionData  = 1
	FbpackSideloadData   = 2
)

var emptyMagic [4]byte
var ErrCommandComplete = errors.New("command complete")

var Version = "development"

func cString(b []byte) string {
	if before, _, ok := bytes.Cut(b, []byte{0}); ok {
		return string(before)
	}
	return string(b)
}

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
				return fmt.Errorf("%s: unexpected magic value: %x", label, provider.GetMagic())
			}
		} else {
			return fmt.Errorf("%s: failed to apply magic provider", label)
		}
	}

	return nil
}

func streamChunks(src io.Reader, dst io.Writer, totalSize uint64, label string, hasher hash.Hash) (int64, error) {
	if label != "" {
		//goland:noinspection GoUnhandledErrorResult
		fmt.Fprintf(os.Stderr, "%s: streaming file ...", label)
	}

	chunk := make([]byte, ChunkSize)
	var totalWritten int64 = 0

	for {
		bytesRead, readErr := src.Read(chunk)
		if bytesRead > 0 {
			if dst != nil {
				bytesWritten, writeErr := dst.Write(chunk[:bytesRead])
				if writeErr != nil {
					if label != "" {
						//goland:noinspection GoUnhandledErrorResult
						fmt.Fprintln(os.Stderr)
					}
					return 0, fmt.Errorf("%s: failed to write data to buffer: %v", label, writeErr)
				}
				totalWritten += int64(bytesWritten)
			} else {
				totalWritten += int64(bytesRead)
			}

			if hasher != nil {
				hasher.Write(chunk[:bytesRead])
			}

			if label != "" {
				//goland:noinspection GoUnhandledErrorResult
				fmt.Fprintf(os.Stderr, "\r\033[2K%s: read %d of %d bytes", label, totalWritten, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if label != "" {
				//goland:noinspection GoUnhandledErrorResult
				fmt.Fprintln(os.Stderr)
			}
			return 0, fmt.Errorf("%s: block read error: %v", label, readErr)
		}
	}

	if label != "" {
		//goland:noinspection GoUnhandledErrorResult
		fmt.Fprintf(os.Stderr, "\r\033[2K%s: successfully written\n", label)
	}

	return totalWritten, nil
}

func fetchFileZip(image Image, offset uint64, end uint64, uncompressedSize uint64, label string, targetFilename string, compressionMethod uint16) (uint32, error) {
	stream, err := image.OpenStream(offset, end)
	if err != nil {
		return 0, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer stream.Close()

	out, err := os.Create(targetFilename)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("%s: failed to create local partition image: %v", label, err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer out.Close()

	var dataReader io.Reader = stream
	if compressionMethod == 8 {
		flateReader := flate.NewReader(stream)
		//goland:noinspection GoUnhandledErrorResult
		defer flateReader.Close()
		dataReader = flateReader
	}

	table := crc32.MakeTable(crc32.IEEE)
	hasher := crc32.New(table)

	_, err = streamChunks(dataReader, out, uncompressedSize, label, hasher)
	if err != nil {
		return 0, err
	}

	return hasher.Sum32(), nil
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
				return fmt.Errorf("erofs: symlinks are not currently supported")
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
			if err != nil {
				return err
			}

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
				} else if chunkIndex.StartBlk != startBlk+uint32(i) {
					return fmt.Errorf("erofs: file blocks are non-contiguous")
				}
			}

			fileOffset := offset + uint64(startBlk)*blockSize
			stream, err := image.OpenStream(fileOffset, fileOffset+fileSize-1)
			if err != nil {
				return err
			}
			defer stream.Close()

			out, err := os.Create(filePath[depth])
			if err != nil {
				fmt.Println()
				return fmt.Errorf("file path: failed to create local file path: %v", err)
			}
			defer out.Close()

			_, err = streamChunks(stream, out, fileSize, "file path", nil)
			return err
		}
	} else {
		return fmt.Errorf("file path: failed to find /%s", strings.Join(filePath[:depth+1], "/"))
	}
}

func fetchInode(image Image, offset uint64, expectedDataLayout uint16) (uint64, uint64, uint64, error) {
	inodeEnd := offset + SizeofErofsInodeExtended + SizeofErofsXattrIbodyHeader + SizeofSharedXattrs - 1
	buf, err := fetchRange(image, offset, inodeEnd)
	if err != nil {
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

func printAvbPropertyDescriptors(image Image, offset uint64, size uint64) error {
	buf, avbHeader, err := findAvbHeader(image, offset, size)
	if err != nil {
		return err
	}

	var avbDescriptor AvbDescriptor
	var avbPropertyDescriptor AvbPropertyDescriptor
	sizeofAvbDescriptor := binary.Size(avbDescriptor)
	sizeofAvbPropertyDescriptor := uint64(binary.Size(avbPropertyDescriptor))

	var descriptorOffset = AvbVBMetaImageHeaderSize + avbHeader.AuthenticationDataBlockSize + avbHeader.DescriptorsOffset
	var descriptorsEnd = descriptorOffset + avbHeader.DescriptorsSize
	for descriptorOffset < descriptorsEnd {
		err := readStruct(buf, descriptorOffset, &avbDescriptor, "avb descriptor", emptyMagic, binary.BigEndian)
		if err != nil {
			return fmt.Errorf("file path: failed to read avb descriptor: %w", err)
		}

		if avbDescriptor.Tag == AvbDescriptorTagProperty {
			err = readStruct(buf, descriptorOffset, &avbPropertyDescriptor, "avb property descriptor", emptyMagic, binary.BigEndian)
			if err != nil {
				return fmt.Errorf("file path: failed to read avb property descriptor: %w", err)
			}
			keyOffset := descriptorOffset + sizeofAvbPropertyDescriptor
			keyEnd := keyOffset + avbPropertyDescriptor.KeyNumBytes
			valueOffset := keyEnd + 1
			valueEnd := valueOffset + avbPropertyDescriptor.ValueNumBytes
			key := string(buf[keyOffset:keyEnd])
			value := string(buf[valueOffset:valueEnd])
			fmt.Printf("%s=%s\n", key, value)
		}

		descriptorOffset += uint64(sizeofAvbDescriptor) + avbDescriptor.NumBytesFollowing
	}
	return nil
}

func verifyPath(targetPath string, targetSize uint64, salt []byte, digest []byte) error {
	hasher := sha256.New()
	hasher.Write(salt)

	file, _, err := openFile(targetPath)
	if err != nil {
		return fmt.Errorf("avb hash descriptor: failed to open target file: %w", err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer file.Close()

	target := io.NewSectionReader(file, 0, int64(targetSize))
	_, err = streamChunks(target, nil, targetSize, "", hasher)
	if err != nil {
		return fmt.Errorf("target image: block read error: %v", err)
	}
	gotDigest := hasher.Sum(nil)

	//goland:noinspection GoRedundantElseInIf
	if !bytes.Equal(gotDigest, digest) {
		return fmt.Errorf("target image: digest mismatch: %x != %x", gotDigest, digest)
	} else {
		fmt.Println("target verified")
		return nil
	}
}

func verifyAvbHashDescriptor(buf []byte, avbHeader AvbVBMetaImageHeader, targetName string, targetPath string) error {
	var avbDescriptor AvbDescriptor
	var avbHashDescriptor AvbHashDescriptor
	sizeofAvbDescriptor := binary.Size(avbDescriptor)
	sizeofAvbHashDescriptor := uint64(binary.Size(avbHashDescriptor))

	var offset = AvbVBMetaImageHeaderSize + avbHeader.AuthenticationDataBlockSize + avbHeader.DescriptorsOffset
	var descriptorsEnd = offset + avbHeader.DescriptorsSize
	for offset < descriptorsEnd {
		err := readStruct(buf, offset, &avbDescriptor, "avb descriptor", emptyMagic, binary.BigEndian)
		if err != nil {
			return fmt.Errorf("file path: failed to read avb descriptor: %w", err)
		}

		if avbDescriptor.Tag == AvbDescriptorTagHash {
			err = readStruct(buf, offset, &avbHashDescriptor, "avb hash descriptor", emptyMagic, binary.BigEndian)
			if err != nil {
				return fmt.Errorf("file path: failed to read avb hash descriptor: %w", err)
			}
			nameOffset := offset + sizeofAvbHashDescriptor
			saltOffset := nameOffset + uint64(avbHashDescriptor.PartitionNameLen)
			digestOffset := saltOffset + uint64(avbHashDescriptor.SaltLen)
			digestEnd := digestOffset + uint64(avbHashDescriptor.DigestLen)
			name := string(buf[nameOffset:saltOffset])
			salt := buf[saltOffset:digestOffset]
			digest := buf[digestOffset:digestEnd]

			if name == targetName {
				hashAlgorithm := cString(avbHashDescriptor.HashAlgorithm[:])
				//goland:noinspection GoRedundantElseInIf
				if hashAlgorithm == "sha256" {
					return verifyPath(targetPath, avbHashDescriptor.ImageSize, salt, digest)
				} else {
					return fmt.Errorf("avb hash descriptor: unexpected hash algorithm: %s", hashAlgorithm)
				}
			}
		}

		offset += uint64(sizeofAvbDescriptor) + avbDescriptor.NumBytesFollowing
	}
	return fmt.Errorf("avb hash descriptor: failed to find target")
}

func findAvbHeader(image Image, offset uint64, size uint64) ([]byte, AvbVBMetaImageHeader, error) {
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
			return nil, avbHeader, err
		}

		offset += avbFooter.VBMetaOffset

		err = fetchStruct(image, offset, &avbHeader, "avb header", avbHeaderMagic, binary.BigEndian)
		if err != nil {
			return nil, avbHeader, err
		}
	}

	avbBuf, err := fetchRange(image, offset, offset+AvbVBMetaImageHeaderSize+avbHeader.AuthenticationDataBlockSize+avbHeader.AuxiliaryDataBlockSize-1)
	if err != nil {
		return nil, avbHeader, err
	}
	return avbBuf, avbHeader, nil
}

func fetchFbpackHeader(image Image, offset uint64) ([]byte, FastBootPackHeader, error) {
	var fbpackHeader FastBootPackHeader
	var fbpackHeaderMagic [4]byte
	copy(fbpackHeaderMagic[:], FbpackMagic)
	err := fetchStruct(image, offset, &fbpackHeader, "fbpack header", fbpackHeaderMagic, binary.LittleEndian)
	if err != nil {
		return nil, fbpackHeader, err
	}

	if fbpackHeader.Version != FbpackVersion {
		return nil, fbpackHeader, fmt.Errorf("unsupported fbpack version")
	}

	fbpackBuf, err := fetchRange(image, offset, offset+uint64(fbpackHeader.TotalSize)-1)
	if err != nil {
		return nil, fbpackHeader, err
	}
	return fbpackBuf, fbpackHeader, nil
}

func readFbpackStructs(image Image, targetPartition string, command string, offset uint64) error {
	fbpackBuf, fbpackHeader, err := fetchFbpackHeader(image, offset)
	if err != nil {
		return err
	}

	var fbpackEntry FastBootPackEntry
	sizeofFbpackHeader := uint64(fbpackHeader.HeaderSize)
	sizeofFbpackEntry := uint64(fbpackHeader.EntryHeaderSize)

	if command == "list-bootloader" {
		fmt.Printf("Header:\n")
		fmt.Printf("magic:              0x%x\n", fbpackHeader.Magic)
		fmt.Printf("version:            %d\n", fbpackHeader.Version)
		fmt.Printf("header size:        %d\n", fbpackHeader.HeaderSize)
		fmt.Printf("entry header size:  %d\n", fbpackHeader.EntryHeaderSize)
		fmt.Printf("platform:           %s\n", cString(fbpackHeader.Platform[:]))
		fmt.Printf("pack version:       %s\n", cString(fbpackHeader.PackVersion[:]))
		fmt.Printf("slot type:          %d\n", fbpackHeader.SlotType)
		fmt.Printf("data align:         %d\n", fbpackHeader.DataAlign)
		fmt.Printf("total entries:      %d\n", fbpackHeader.TotalEntries)
		fmt.Printf("total size:         %d\n", fbpackHeader.TotalSize)
		fmt.Printf("\nEntries:\n")
	}

	var entryOffset = sizeofFbpackHeader
	var entriesEnd = entryOffset + uint64(fbpackHeader.TotalSize)
	for i := range fbpackHeader.TotalEntries {
		err := readStruct(fbpackBuf, entryOffset, &fbpackEntry, "fbpack entry", emptyMagic, binary.LittleEndian)
		if err != nil {
			return fmt.Errorf("file path: failed to read fbpack entry: %w", err)
		}
		name := cString(fbpackEntry.Name[:])

		if command == "list-bootloader" {
			fmt.Printf("offset: %d\n", entryOffset)
			fmt.Printf("Entry %d {\n", i+1)
			fmt.Printf("    name:       %s\n", name)
			eType := "unknown"
			if fbpackEntry.Type == FbpackPartitionTable {
				eType = "partition table"
			} else if fbpackEntry.Type == FbpackPartitionData {
				eType = "partition data"
			} else if fbpackEntry.Type == FbpackSideloadData {
				eType = "sideload"
			}
			fmt.Printf("    type:       %s\n", eType)
			fmt.Printf("    product:    %s\n", cString(fbpackEntry.Product[:]))
			fmt.Printf("    offset:     0x%x (%d)\n", fbpackEntry.Offset, fbpackEntry.Offset)
			fmt.Printf("    size:       0x%x (%d)\n", fbpackEntry.Size, fbpackEntry.Size)
			fmt.Printf("    slotted:    %t\n", fbpackEntry.Slotted != 0)
			fmt.Printf("    crc32:      0x%08x\n", fbpackEntry.Crc32)
			fmt.Printf("}\n")
		} else if name == targetPartition {
			// allows defer in a loop
			return func() error {
				//goland:noinspection GoRedundantElseInIf
				if command == "list-bootloader-ar" {
					return printBootloaderAr(image, offset+fbpackEntry.Offset)
				} else if command == "extract-bootloader" {
					stream, err := image.OpenStream(offset+fbpackEntry.Offset, offset+fbpackEntry.Offset+fbpackEntry.Size-1)
					if err != nil {
						return err
					}
					//goland:noinspection GoUnhandledErrorResult
					defer stream.Close()

					out, err := os.Create(fmt.Sprintf("%s.img", name))
					if err != nil {
						fmt.Println()
						return fmt.Errorf("file path: failed to create local file path: %v", err)
					}
					//goland:noinspection GoUnhandledErrorResult
					defer out.Close()

					_, err = streamChunks(stream, out, fbpackEntry.Size, "file path", nil)
					return err
				} else {
					return fmt.Errorf("unexpected command: %s", command)
				}
			}()
		}

		entryOffset += sizeofFbpackEntry
		if entryOffset >= entriesEnd {
			break
		}
	}

	if command == "list-bootloader-ar" || command == "extract-bootloader" {
		return fmt.Errorf("file path: failed to find %s", targetPartition)
	}

	return nil
}

func printAvbRollbackIndex(image Image, offset uint64, size uint64) error {
	_, avbHeader, err := findAvbHeader(image, offset, size)
	if err != nil {
		return err
	}
	fmt.Printf("%d\n", avbHeader.RollbackIndex)
	return nil
}

func printBootloaderAr(image Image, offset uint64) error {
	buf, err := fetchRange(image, offset, offset+4-1)
	if err != nil {
		return err
	}

	version := binary.LittleEndian.Uint32(buf[:4])

	var ar uint32
	// var payloadSize uint32
	// var hashLength uint32
	var sec bool

	descriptorOffset := uint64(0x400)
	// hashOffset := uint64(0x20)
	// payloadOffset := uint64(0x1000)

	buf, err = fetchRange(image, offset+descriptorOffset, offset+descriptorOffset+96-1)
	var bootloaderHeader BootloaderDescriptor
	err = readStruct(buf, 0, &bootloaderHeader, "bootloader descriptor", emptyMagic, binary.LittleEndian)
	if err != nil {
		return err
	}

	if bytes.Equal(bootloaderHeader.Magic[:], []byte("ABL\x00")) || bytes.Equal(bootloaderHeader.Magic[:], []byte("APBL")) {
		if bytes.Equal(bootloaderHeader.Magic[:], []byte("APBL")) {
			sec = true
		}

		ar = bootloaderHeader.Ar
		if sec {
			fmt.Printf("sec_ar=%d\n", ar)
		} else {
			fmt.Printf("nonsec_ar=%d\n", ar)
		}

		// hashLength = 32 // Tensor G1-G2 (raven, oriole, bluejay, panther, cheetah, lynx, tangorpro, felix)
		// for i := hashOffset + 32; i < hashOffset+64; i++ {
		// 	if buf[i] != 0 {
		// 		hashLength = 64 // Tensor G3-G4 (shiba, husky, akita, tokay, komodo, caiman, comet, tegu)
		// 		break
		// 	}
		// }
	} else {
		if version == 1 {
			descriptorOffset = 0x0864 // Tensor G5 (mustang, frankel, blazer, rango)
		} else if version == 2 {
			descriptorOffset = 0x463C // Tensor G6 (stallion, yogi, kodiak, grizzly, cubs)
			// payloadOffset = 0x5000
		} else {
			return fmt.Errorf("bootloader descriptor: unexpected magic and version: 0x%x, %d", binary.LittleEndian.Uint32(bootloaderHeader.Magic[:]), version)
		}

		buf, err = fetchRange(image, offset+descriptorOffset, offset+descriptorOffset+104-1)
		if err != nil {
			return err
		}

		var bootloaderHeaderG5 BootloaderDescriptorG5
		err = readStruct(buf, 0, &bootloaderHeaderG5, "bootloader descriptor", emptyMagic, binary.LittleEndian)
		if err != nil {
			return err
		}

		if bytes.Equal(bootloaderHeaderG5.Magic[:], []byte("APBL")) || bytes.Equal(bootloaderHeaderG5.Magic[:], []byte("LBSG")) {
			if bytes.Equal(bootloaderHeaderG5.Magic[:], []byte("LBSG")) {
				sec = true
			}
			// hashOffset = 0x38
			// hashLength = 48

			ar = bootloaderHeaderG5.Ar
			if sec {
				fmt.Printf("sec_ar=%d\n", ar)
			} else {
				fmt.Printf("nonsec_ar=%d\n", ar)
			}
		} else {
			return fmt.Errorf("bootloader descriptor: unexpected magic and version: 0x%x, %d", binary.LittleEndian.Uint32(bootloaderHeaderG5.Magic[:]), version)
		}
	}

	// hashBuf, err := fetchRange(image, descriptorOffset+uint64(hashOffset), descriptorOffset+uint64(hashOffset)+uint64(hashLength)-1)
	// if err != nil {
	// 	return err
	// }
	//
	// payloadChunk, err := fetchRange(image, payloadOffset, payloadOffset+uint64(payloadSize)-1)
	// if err != nil {
	// 	return err
	// }
	//
	// var digest []byte
	// if hashLength == 64 {
	// 	sum := sha512.Sum512(payloadChunk)
	// 	digest = sum[:]
	// } else if hashLength == 48 {
	// 	sum := sha512.Sum384(payloadChunk)
	// 	digest = sum[:]
	// } else {
	// 	sum := sha256.Sum256(payloadChunk)
	// 	digest = sum[:]
	// }
	//
	// if bytes.Equal(digest, hashBuf) {
	// 	fmt.Println("bootloader descriptor: payload hash verified successfully!")
	// } else {
	// 	return fmt.Errorf("bootloader descriptor: payload hash verification failed")
	// }

	return nil
}

func findAndReadCentralDirectory(image Image, partitionFilename string, command string, subcommand string, subcommandArgs []string, sofOffset uint64, eofOffset uint64) error {
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
	var partitionRegex *regexp.Regexp
	if partitionFilename == "bootloader.img" {
		partitionRegex = regexp.MustCompile(`.+/bootloader-.+\.img`)
	} else {
		partitionRegex = regexp.MustCompile(fmt.Sprintf(`(^|/)%s`, regexp.QuoteMeta(partitionFilename)))
	}

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

				//goland:noinspection GoUnhandledErrorResult
				binary.Read(extraReader, binary.LittleEndian, &headerID)
				//goland:noinspection GoUnhandledErrorResult
				binary.Read(extraReader, binary.LittleEndian, &dataSize)

				if headerID == 0x0001 {
					if cdfHeader.UncompressedSize == 0xFFFFFFFF {
						//goland:noinspection GoUnhandledErrorResult
						binary.Read(extraReader, binary.LittleEndian, &uncompressedSize64)
						uncompressedSize = uncompressedSize64
					}
					if cdfHeader.CompressedSize == 0xFFFFFFFF {
						//goland:noinspection GoUnhandledErrorResult
						binary.Read(extraReader, binary.LittleEndian, &compressedSize64)
						compressedSize = compressedSize64
					}
					if cdfHeader.LfhOffset == 0xFFFFFFFF {
						//goland:noinspection GoUnhandledErrorResult
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

		if command == "list-factory" {
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
				if command == "extract-factory" || command == "passthrough-factory" || command == "list-factory" {
					err = findAndReadCentralDirectory(image, partitionFilename, command, subcommand, subcommandArgs, lfOffset, lfOffset+compressedSize)
					if err != nil {
						return err
					}
				}
			} else {
				//goland:noinspection GoRedundantElseInIf
				// allows defer in a loop
				return func() error {
					var label string
					var subImage Image
					var subOffset uint64

					if cdfHeader.CompressionMethod == 8 || slices.Contains([]string{"extract-factory", "extract-zip"}, command) {
						var targetFilename string
						if !slices.Contains([]string{"extract-factory", "extract-zip"}, command) {
							label = "tmp file"
							tmpFile, err := os.CreateTemp("", "pixel-plucker-*.img")
							if err != nil {
								//goland:noinspection GoUnhandledErrorResult
								tmpFile.Close()
								//goland:noinspection GoUnhandledErrorResult
								os.Remove(tmpFile.Name())
								return fmt.Errorf("central directory: failed to create temp file: %w", err)
							}
							//goland:noinspection GoUnhandledErrorResult
							defer os.Remove(tmpFile.Name())
							//goland:noinspection GoUnhandledErrorResult
							defer fmt.Fprintf(os.Stderr, "%s: file removed\n", label)
							targetFilename = tmpFile.Name()
							//goland:noinspection GoUnhandledErrorResult
							tmpFile.Close()
						} else {
							label = "partition image"
							targetFilename = partitionFilename
						}

						hashCrc32, err := fetchFileZip(image, lfOffset, lfOffset+compressedSize-1, uncompressedSize, label, targetFilename, cdfHeader.CompressionMethod)
						if err != nil {
							return err
						}
						if hashCrc32 != cdfHeader.Crc32 {
							//goland:noinspection GoUnhandledErrorResult
							os.Remove(partitionFilename)
							return fmt.Errorf("central directory: hash mismatch")
						} else {
							file, err := os.Open(targetFilename)
							if err != nil {
								return fmt.Errorf("%s: failed to open: %w", label, err)
							}
							//goland:noinspection GoUnhandledErrorResult
							defer file.Close()
							subImage = &LocalImage{file: file}
							subOffset = 0
						}
					} else {
						subImage = image
						subOffset = lfOffset
					}

					switch command {
					case "extract-factory", "extract-zip":
						// do nothing, file already extracted
					case "passthrough-factory":
						switch subcommand {
						case "extract-erofs":
							filePath := subcommandArgs[0]
							err = findAndExtractFile(subImage, filePath, subOffset)

						case "extract-bootloader":
							partitionName := subcommandArgs[0]
							err = readFbpackStructs(subImage, partitionName, subcommand, subOffset)

						case "list-bootloader":
							err = readFbpackStructs(subImage, "", subcommand, subOffset)

						case "list-bootloader-ar":
							partitionName := subcommandArgs[0]
							err = readFbpackStructs(subImage, partitionName, subcommand, subOffset)

						case "list-bootloader-image-ar":
							err = printBootloaderAr(subImage, subOffset)

						case "list-avb-props":
							err = printAvbPropertyDescriptors(subImage, subOffset, uncompressedSize)

						case "list-avb-rollback-index":
							err = printAvbRollbackIndex(subImage, subOffset, uncompressedSize)

						default:
							err = fmt.Errorf("unexpected subcommand: %s", subcommand)
						}
					default:
						err = fmt.Errorf("unexpected command: %s", command)
					}

					if err != nil {
						return err
					} else {
						return ErrCommandComplete
					}
				}()
			}
		}

		offset += sizeofCdfHeader + n + m + k
	}

	if command != "list-factory" {
		//goland:noinspection GoRedundantElseInIf
		if sofOffset == 0 {
			return fmt.Errorf("failed to find nested image zip")
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

func openFile(filename string) (*os.File, int64, error) {
	var imageSize int64

	file, err := os.Open(filename)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open local file: %v\n", err)
	}

	info, err := file.Stat()
	if err != nil {
		//goland:noinspection GoUnhandledErrorResult
		file.Close()
		return nil, 0, fmt.Errorf("failed to stat local file: %v\n", err)
	}

	mode := info.Mode()
	if mode.IsRegular() {
		imageSize = info.Size()
	} else if mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0 {
		imageSize, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			//goland:noinspection GoUnhandledErrorResult
			file.Close()
			return nil, 0, fmt.Errorf("failed to seek local block device: %v\n", err)
		}
	} else {
		//goland:noinspection GoUnhandledErrorResult
		file.Close()
		return nil, 0, fmt.Errorf("unexpected local file type")
	}

	return file, imageSize, nil
}

//goland:noinspection GoUnhandledErrorResult
func showUsage() {
	fmt.Fprintln(os.Stderr, "Usage: pluck <command> [arguments]")
	fmt.Fprintln(os.Stderr, "\nCommands:")
	fmt.Fprintln(os.Stderr, "  extract-factory           Extract partition image from factory image")
	fmt.Fprintln(os.Stderr, "  passthrough-factory       Pass partition image from factory image through to subcommand")
	fmt.Fprintln(os.Stderr, "  extract-zip               Extract target file from zip file")
	fmt.Fprintln(os.Stderr, "  extract-erofs             Extract target file from erofs partition image")
	fmt.Fprintln(os.Stderr, "  extract-bootloader        Extract partition image from factory bootloader image")
	fmt.Fprintln(os.Stderr, "  list-factory              List factory zip contents")
	fmt.Fprintln(os.Stderr, "  list-bootloader           List factory bootloader image entries")
	fmt.Fprintln(os.Stderr, "  list-bootloader-ar        Print anti-rollback value from factory bootloader image entry")
	fmt.Fprintln(os.Stderr, "  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image")
	fmt.Fprintln(os.Stderr, "  list-avb-props            List AVB property descriptors")
	fmt.Fprintln(os.Stderr, "  list-avb-rollback-index   Print AVB rollback index value")
	fmt.Fprintln(os.Stderr, "  verify-avb-hash           Verify AVB hash descriptor against partition image")
	fmt.Fprintln(os.Stderr, "  version                   Print version and exit")
	fmt.Fprintln(os.Stderr, "  help                      Show usage")
}

//goland:noinspection GoUnhandledErrorResult
func showCommandUsage(command string) {
	switch command {
	case "extract-factory":
		fmt.Fprintln(os.Stderr, "Usage: pluck extract-factory <factoryImageUrlOrFile> <partitionFilename>")
		fmt.Fprintln(os.Stderr, "\nExtract partition from factory image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  factoryImageUrlOrFile     Remote URL or local file path of factory image")
		fmt.Fprintln(os.Stderr, "  partitionFilename         Name of partition image to extract")
	case "passthrough-factory":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> <subcommand> [arguments]")
		fmt.Fprintln(os.Stderr, "\nPass partition image from factory image through to subcommand")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  factoryImageUrlOrFile     Remote URL or local file path of factory image")
		fmt.Fprintln(os.Stderr, "  partitionFilename         Name of partition image to pass through")
		fmt.Fprintln(os.Stderr, "  subcommand                Subcommand to handle partition image")
		fmt.Fprintln(os.Stderr, "\nSupported subcommands:")
		fmt.Fprintln(os.Stderr, "  extract-erofs             Extract target file from erofs partition image")
		fmt.Fprintln(os.Stderr, "  extract-bootloader        Extract partition image from bootloader image")
		fmt.Fprintln(os.Stderr, "  list-bootloader           List bootloader image entries")
		fmt.Fprintln(os.Stderr, "  list-bootloader-ar        Print anti-rollback value from bootloader image entry")
		fmt.Fprintln(os.Stderr, "  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image")
		fmt.Fprintln(os.Stderr, "  list-avb-props            List AVB properties")
		fmt.Fprintln(os.Stderr, "  list-avb-rollback-index   Print AVB rollback index")
		fmt.Fprintln(os.Stderr, "  help                      Show usage")
	case "extract-zip":
		fmt.Fprintln(os.Stderr, "Usage: pluck extract-zip <zipUrlOrFile> <targetFilename>")
		fmt.Fprintln(os.Stderr, "\nExtract target file from zip file")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  zipUrlOrFile              Remote URL or local file path of zip file")
		fmt.Fprintln(os.Stderr, "  filePath                  Path to file in zip file to extract")
	case "extract-erofs":
		fmt.Fprintln(os.Stderr, "Usage: pluck extract-erofs <partitionImageUrlOrFile> <filePath>")
		fmt.Fprintln(os.Stderr, "\nExtract target file from erofs partition image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionImageUrlOrFile   Remote URL or local file path of partition image")
		fmt.Fprintln(os.Stderr, "  filePath                  Path to file in partition image to extract")
	case "extract-bootloader":
		fmt.Fprintln(os.Stderr, "Usage: pluck extract-bootloader <bootloaderImageUrlOrFile> <partitionName>")
		fmt.Fprintln(os.Stderr, "\nExtract partition image from factory bootloader image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image")
		fmt.Fprintln(os.Stderr, "  partitionName             Name of partition image to extract")
	case "list-factory":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-factory <factoryImageUrlOrFile>")
		fmt.Fprintln(os.Stderr, "\nList factory zip contents")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  factoryImageUrlOrFile     Remote URL or local file path of factory image")
	case "list-bootloader":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-bootloader <bootloaderImageUrlOrFile>")
		fmt.Fprintln(os.Stderr, "\nList factory bootloader image entries")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image")
	case "list-bootloader-ar":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-bootloader-ar <bootloaderImageUrlOrFile> <partitionName>")
		fmt.Fprintln(os.Stderr, "\nPrint anti-rollback value from factory bootloader image entry")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  bootloaderImageUrlOrFile  Remote URL or local file path of factory bootloader image")
		fmt.Fprintln(os.Stderr, "  partitionName             Name of partition containing anti-rollback value")
	case "list-bootloader-image-ar":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-bootloader-image-ar <partitionImageUrlOrFile>")
		fmt.Fprintln(os.Stderr, "\nPrint anti-rollback value from bootloader partition image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionImageUrlOrFile   Remote URL or local file/device path of partition image")
	case "list-avb-props":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-avb-props <partitionImageUrlOrFile>")
		fmt.Fprintln(os.Stderr, "\nList AVB property descriptors")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionImageUrlOrFile   Remote URL or local file/device path of partition image")
	case "list-avb-rollback-index":
		fmt.Fprintln(os.Stderr, "Usage: pluck list-avb-rollback-index <partitionImageUrlOrFile>")
		fmt.Fprintln(os.Stderr, "\nPrint AVB rollback index value")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionImageUrlOrFile   Remote URL or local file/device path of partition image")
	case "verify-avb-hash":
		fmt.Fprintln(os.Stderr, "Usage: pluck verify-avb-hash <vbmetaImageFile> <targetName> <targetPath>")
		fmt.Fprintln(os.Stderr, "\nVerify AVB hash descriptor against file/device")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  vbmetaImageFile           Local file/device path of vbmeta image")
		fmt.Fprintln(os.Stderr, "  targetName                Name of partition to verify")
		fmt.Fprintln(os.Stderr, "  targetPath                Local file/device path of partition to verify")
	case "version":
		fmt.Fprintln(os.Stderr, "Usage: pluck version")
		fmt.Fprintln(os.Stderr, "\nPrint version and exit")
	case "help", "--help", "-h":
		fmt.Fprintln(os.Stderr, "Usage: pluck help [command]")
		fmt.Fprintln(os.Stderr, "\nShow usage")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  command                   Command to show usage of (optional)")
		fmt.Fprintln(os.Stderr, "\nSupported commands:")
		fmt.Fprintln(os.Stderr, "  extract-factory           Extract partition image from factory image")
		fmt.Fprintln(os.Stderr, "  passthrough-factory       Pass partition image from factory image through to subcommand")
		fmt.Fprintln(os.Stderr, "  extract-zip               Extract target file from zip file")
		fmt.Fprintln(os.Stderr, "  extract-erofs             Extract target file from erofs partition image")
		fmt.Fprintln(os.Stderr, "  extract-bootloader        Extract partition image from factory bootloader image")
		fmt.Fprintln(os.Stderr, "  list-factory              List factory zip contents")
		fmt.Fprintln(os.Stderr, "  list-bootloader           List factory bootloader image entries")
		fmt.Fprintln(os.Stderr, "  list-bootloader-ar        Print anti-rollback value from factory bootloader image entry")
		fmt.Fprintln(os.Stderr, "  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image")
		fmt.Fprintln(os.Stderr, "  list-avb-props            List AVB property descriptors")
		fmt.Fprintln(os.Stderr, "  list-avb-rollback-index   Print AVB rollback index value")
		fmt.Fprintln(os.Stderr, "  verify-avb-hash           Verify AVB hash descriptor against partition image")
		fmt.Fprintln(os.Stderr, "  version                   Print version and exit")
		fmt.Fprintln(os.Stderr, "  help                      Show this usage")
	default:
		fmt.Fprintf(os.Stderr, "Unknown or unsupported command: %s\n", command)
	}
}

//goland:noinspection GoUnhandledErrorResult
func showPassthroughFactorySubcommandUsage(subcommand string) {
	switch subcommand {
	case "extract-erofs":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> extract-erofs <filePath>")
		fmt.Fprintln(os.Stderr, "\nExtract target file from erofs partition image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  filePath                  Path to file in partition image to extract")
	case "extract-bootloader":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> extract-bootloader <partitionName>")
		fmt.Fprintln(os.Stderr, "\nExtract partition image from bootloader image")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionName             Name of partition image to extract")
	case "list-bootloader":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> bootloader.img list-bootloader")
		fmt.Fprintln(os.Stderr, "\nList bootloader image entries")
	case "list-bootloader-ar":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> bootloader.img list-bootloader-ar <partitionName>")
		fmt.Fprintln(os.Stderr, "\nPrint anti-rollback value from bootloader image entry")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  partitionName             Name of partition containing anti-rollback value")
	case "list-bootloader-image-ar":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> list-bootloader-image-ar")
		fmt.Fprintln(os.Stderr, "\nPrint anti-rollback value from bootloader partition image")
	case "list-avb-props":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> list-avb-props")
		fmt.Fprintln(os.Stderr, "\nList AVB property descriptors")
	case "list-avb-rollback-index":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> list-avb-rollback-index")
		fmt.Fprintln(os.Stderr, "\nPrint AVB rollback index value")
	case "help", "--help", "-h":
		fmt.Fprintln(os.Stderr, "Usage: pluck passthrough-factory <factoryImageUrlOrFile> <partitionFilename> help <subcommand>")
		fmt.Fprintln(os.Stderr, "\nShow usage")
		fmt.Fprintln(os.Stderr, "\nArguments:")
		fmt.Fprintln(os.Stderr, "  subcommand                Subcommand to show usage of")
		fmt.Fprintln(os.Stderr, "\nSupported subcommands:")
		fmt.Fprintln(os.Stderr, "  extract-erofs             Extract target file from partition image")
		fmt.Fprintln(os.Stderr, "  extract-bootloader        Extract partition image from bootloader image")
		fmt.Fprintln(os.Stderr, "  list-bootloader           List factory bootloader image entries")
		fmt.Fprintln(os.Stderr, "  list-bootloader-ar        Print anti-rollback value from factory bootloader image entry")
		fmt.Fprintln(os.Stderr, "  list-bootloader-image-ar  Print anti-rollback value from bootloader partition image")
		fmt.Fprintln(os.Stderr, "  list-avb-props            List AVB properties")
		fmt.Fprintln(os.Stderr, "  list-avb-rollback-index   Print AVB rollback index")
		fmt.Fprintln(os.Stderr, "  help                      Show this usage")
	default:
		fmt.Fprintf(os.Stderr, "Unknown or unsupported subcommand: %s\n", subcommand)
	}
}

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "version", "--version":
		fmt.Printf("pluck %s\n", Version)
		os.Exit(0)
	case "help", "--help", "-h":
		if len(args) > 0 {
			showCommandUsage(args[0])
		} else {
			showUsage()
		}
		os.Exit(0)
	}

	var image Image
	var imageSize int64

	urlOrFile := args[0]
	//goland:noinspection HttpUrlsUsage
	if strings.HasPrefix(urlOrFile, "http://") || strings.HasPrefix(urlOrFile, "https://") {
		client := &http.Client{}
		res, err := client.Head(urlOrFile)
		if err != nil {
			//goland:noinspection GoUnhandledErrorResult
			fmt.Fprintf(os.Stderr, "factory image: HEAD request failed: %v\n", err)
			os.Exit(1)
		}
		//goland:noinspection GoUnhandledErrorResult
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			//goland:noinspection GoUnhandledErrorResult
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
			//goland:noinspection GoUnhandledErrorResult
			fmt.Fprintf(os.Stderr, "factory image: failed to parse Content-Length: %v\n", err)
			os.Exit(1)
		}
	} else {
		var file *os.File
		var err error
		file, imageSize, err = openFile(urlOrFile)
		if err != nil {
			//goland:noinspection GoUnhandledErrorResult
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		image = &LocalImage{file: file}
	}
	//goland:noinspection GoUnhandledErrorResult
	defer image.Close()

	var err error
	switch command {
	case "extract-factory":
		if len(args) != 2 {
			showCommandUsage(command)
			os.Exit(1)
		}
		partitionFilename := args[1]
		err = findAndReadCentralDirectory(image, partitionFilename, command, "", nil, 0, uint64(imageSize))

	case "passthrough-factory":
		if len(args) < 3 {
			showCommandUsage(command)
			os.Exit(1)
		}
		partitionFilename := args[1]
		subcommand := args[2]
		subcommandArgs := args[3:]

		switch subcommand {
		case "help", "--help", "-h":
			if len(subcommandArgs) > 0 {
				showPassthroughFactorySubcommandUsage(subcommandArgs[0])
			} else {
				showCommandUsage(command)
			}

		default:
			switch subcommand {
			case "extract-erofs", "extract-bootloader", "list-bootloader-ar":
				if len(subcommandArgs) != 1 {
					showPassthroughFactorySubcommandUsage(subcommand)
					os.Exit(1)
				}
			case "list-bootloader", "list-bootloader-image-ar", "list-avb-props", "list-avb-rollback-index":
				if len(subcommandArgs) != 0 {
					showPassthroughFactorySubcommandUsage(subcommand)
					os.Exit(1)
				}
			default:
				//goland:noinspection GoUnhandledErrorResult
				fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
				showCommandUsage(command)
				os.Exit(1)
			}

			err = findAndReadCentralDirectory(image, partitionFilename, command, subcommand, subcommandArgs, 0, uint64(imageSize))
		}

	case "extract-zip":
		if len(args) != 2 {
			showCommandUsage(command)
			os.Exit(1)
		}
		targetFilename := args[1]
		err = findAndReadCentralDirectory(image, targetFilename, command, "", nil, 0, uint64(imageSize))

	case "extract-erofs":
		if len(args) != 2 {
			showCommandUsage(command)
			os.Exit(1)
		}
		filePath := args[1]
		err = findAndExtractFile(image, filePath, 0)

	case "extract-bootloader":
		if len(args) != 2 {
			showCommandUsage(command)
			os.Exit(1)
		}
		targetPartition := args[1]
		err = readFbpackStructs(image, targetPartition, command, 0)

	case "list-factory":
		if len(args) != 1 {
			showCommandUsage(command)
			os.Exit(1)
		}
		err = findAndReadCentralDirectory(image, "", command, "", nil, 0, uint64(imageSize))

	case "list-bootloader":
		if len(args) != 1 {
			showCommandUsage(command)
			os.Exit(1)
		}
		err = readFbpackStructs(image, "", command, 0)

	case "list-bootloader-ar":
		if len(args) != 2 {
			showCommandUsage(command)
			os.Exit(1)
		}
		targetPartition := args[1]
		err = readFbpackStructs(image, targetPartition, command, 0)

	case "list-bootloader-image-ar":
		if len(args) != 1 {
			showCommandUsage(command)
			os.Exit(1)
		}
		err = printBootloaderAr(image, 0)

	case "list-avb-props":
		if len(args) != 1 {
			showCommandUsage(command)
			os.Exit(1)
		}
		err = printAvbPropertyDescriptors(image, 0, uint64(imageSize))

	case "list-avb-rollback-index":
		if len(args) != 1 {
			showCommandUsage(command)
			os.Exit(1)
		}
		err = printAvbRollbackIndex(image, 0, uint64(imageSize))

	case "verify-avb-hash":
		if len(args) != 3 {
			showCommandUsage(command)
			os.Exit(1)
		}
		avbBuf, avbHeader, hErr := findAvbHeader(image, 0, uint64(imageSize))
		if hErr != nil {
			err = hErr
		} else {
			targetName := args[1]
			targetPath := args[2]
			err = verifyAvbHashDescriptor(avbBuf, avbHeader, targetName, targetPath)
		}

	default:
		//goland:noinspection GoUnhandledErrorResult
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", command)
		showUsage()
		os.Exit(1)
	}

	if err != nil && !errors.Is(err, ErrCommandComplete) {
		//goland:noinspection GoUnhandledErrorResult
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
