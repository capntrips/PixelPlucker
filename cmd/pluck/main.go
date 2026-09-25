package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
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

//goland:noinspection GoUnusedExportedType
type ErofsInodeCompact struct {
	Format     uint16
	XattrCount uint16
	Mode       uint16
	Nlink      uint16
	Size       uint32
	Reserved   uint32
	InodeData  uint32
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
	InodeData  uint32
	Inode      uint32
	Uid        uint32
	Gid        uint32
	Mtime      uint64
	MtimeNs    uint32
	Nlink      uint32
}

type ErofsDirent struct {
	Nid      uint64
	NameOff  uint16
	FileType uint8
	Reserved uint8
}

type ErofsInodeChunkIndex struct {
	StartBlkHi uint16
	DeviceID   uint16
	StartBlkLo uint32
}

type MagicProvider interface {
	GetMagic() [4]byte
}

func (h LocalFileHeader) GetMagic() [4]byte  { return h.Magic }
func (h CdFileHeader) GetMagic() [4]byte     { return h.Magic }
func (r EocdRecord) GetMagic() [4]byte       { return r.Magic }
func (l Zip64EocdLocator) GetMagic() [4]byte { return l.Magic }
func (r Zip64EocdRecord) GetMagic() [4]byte  { return r.Magic }
func (s ErofsSuperblock) GetMagic() [4]byte  { return s.Magic }
func (s SparseStub) GetMagic() [4]byte       { return s.Magic }
func (s ErofsStub) GetMagic() [4]byte        { return s.Magic }

const (
	ChunkSize = 16384

	// https://erofs.docs.kernel.org/en/latest/ondisk/core_ondisk.html

	ErofsFtRegFile = 1
	ErofsFtDir     = 2
	ErofsFtSymlink = 7

	// https://android.googlesource.com/kernel/common/+/refs/heads/android17-6.18-2026-09/fs/erofs/erofs_fs.h

	ErofsINodeFlatPlain  = 0
	ErofsIDatalayoutMask = uint16(0x07)
)

var emptyMagic [4]byte

var Version = "development"

//goland:noinspection GoUnhandledErrorResult
func fetchRange(url string, offset uint64, end uint64, label string, client *http.Client) ([]byte, error) {
	empty := make([]byte, 0, end-offset)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
	res, err := client.Do(req)
	if err != nil {
		return empty, fmt.Errorf("%s: GET request failed: %w", label, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		return empty, fmt.Errorf("%s: GET request rejected with status: %s", label, res.Status)
	}

	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return empty, fmt.Errorf("%s: failed to read response: %w", label, err)
	}

	return buf, nil
}

func fetchStruct(url string, offset uint64, target any, label string, magic [4]byte, client *http.Client) error {
	sizeofStruct := binary.Size(target)
	end := offset + uint64(sizeofStruct) - 1

	buf, err := fetchRange(url, offset, end, label, client)
	if err != nil {
		return err
	}

	return readStruct(buf, 0, target, label, magic)
}

func readStruct(buf []byte, offset uint64, target any, label string, magic [4]byte) error {
	reader := bytes.NewReader(buf[offset:])
	if err := binary.Read(reader, binary.LittleEndian, target); err != nil {
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
func fetchFileZip(url string, offset uint64, end uint64, uncompressedSize uint64, partitionFilename string, compressionMethod uint16, client *http.Client) (uint32, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))

	fmt.Print("partition image: downloading file ...")
	res, err := client.Do(req)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("partition image: GET request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		fmt.Println()
		return 0, fmt.Errorf("partition image: GET request rejected with status: %s", res.Status)
	}

	out, err := os.Create(partitionFilename)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("partition image: failed to create local partition image: %v", err)
	}
	defer out.Close()

	var dataReader io.Reader = res.Body
	if compressionMethod == 8 {
		flateReader := flate.NewReader(res.Body)
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
			return 0, fmt.Errorf("partition image: GET request block read error: %v", readErr)
		}
	}

	fmt.Println("\r\033[2Kpartition image: successfully downloaded")

	return hash.Sum32(), nil
}

func fetchFileErofs(url string, offset uint64, nid uint64, filePath []string, depth int, superblock ErofsSuperblock, client *http.Client) error {
	dirents, err := fetchInodeDirents(nid, url, offset, superblock, client)
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
			return fetchFileErofs(url, offset, dirent.Nid, filePath, depth+1, superblock, client)
		} else {
			if dirent.FileType != ErofsFtRegFile {
				return fmt.Errorf("erofs: unexpected file but got unexpected type: %d", dirent.FileType)
			}
			blockSize := uint64(1) << superblock.BlkSizeBits
			nidOffset := uint64(superblock.MetaBlkAddr)*blockSize + dirent.Nid*32

			var inode ErofsInodeExtended
			if err = fetchStruct(url, offset+nidOffset, &inode, "extended inode", emptyMagic, client); err != nil {
				return err
			}

			dataLayout := (inode.Format & ErofsIDatalayoutMask) >> 1
			if dataLayout != ErofsINodeFlatPlain {
				return fmt.Errorf("erofs: unexpected file datalayout: %d", dirent.FileType)
			}

			sizeofInode := uint64(binary.Size(inode))
			chunkIndexOffset := offset + nidOffset + sizeofInode
			var chunkIndex ErofsInodeChunkIndex
			if err = fetchStruct(url, chunkIndexOffset, &chunkIndex, "chunk index", emptyMagic, client); err != nil {
				return err
			}

			fileOffset := offset + uint64(chunkIndex.StartBlkHi)*blockSize

			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", fileOffset, fileOffset+inode.Size-1))

			fmt.Print("file path: downloading file ...")
			res, err := client.Do(req)
			if err != nil {
				fmt.Println()
				return fmt.Errorf("file path: GET request failed: %v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusPartialContent {
				fmt.Println()
				return fmt.Errorf("file path: GET request rejected with status: %s", res.Status)
			}

			out, err := os.Create(filePath[depth])
			if err != nil {
				fmt.Println()
				return fmt.Errorf("file path: failed to create local file path: %v", err)
			}
			defer out.Close()

			chunk := make([]byte, ChunkSize)
			var totalWritten int64 = 0

			for {
				bytesRead, readErr := res.Body.Read(chunk)
				if bytesRead > 0 {
					bytesWritten, writeErr := out.Write(chunk[:bytesRead])
					if writeErr != nil {
						fmt.Println()
						return fmt.Errorf("file path: failed to write data to buffer: %v", writeErr)
					}
					totalWritten += int64(bytesWritten)
					fmt.Printf("\r\033[2Kfile path: read %d of %d bytes", totalWritten, inode.Size)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					fmt.Println()
					return fmt.Errorf("file path: GET request block read error: %v", readErr)
				}
			}

			fmt.Println("\r\033[2Kfile path: successfully downloaded")

			return nil
		}
	} else {
		return fmt.Errorf("file path: failed to find /%s", strings.Join(filePath[:depth+1], "/"))
	}
}

//goland:noinspection GoUnhandledErrorResult
func fetchInodeDirents(nid uint64, url string, offset uint64, superblock ErofsSuperblock, client *http.Client) (map[string]ErofsDirent, error) {
	blockSize := uint64(1) << superblock.BlkSizeBits
	nidOffset := uint64(superblock.MetaBlkAddr)*blockSize + nid*32

	var inode ErofsInodeExtended
	if err := fetchStruct(url, offset+nidOffset, &inode, "extended inode", emptyMagic, client); err != nil {
		return nil, err
	}

	sizeofInode := uint64(binary.Size(inode))
	direntOffset := offset + nidOffset + sizeofInode
	buf, err := fetchRange(url, direntOffset, direntOffset+inode.Size, "dirents", client)
	if err != nil {
		return nil, err
	}

	fileMap := make(map[string]ErofsDirent)

	var dirent ErofsDirent
	err = readStruct(buf, 0, &dirent, "dirent", emptyMagic)
	if err != nil {
		return nil, fmt.Errorf("dirents: failed to read initial dirent: %w", err)
	}

	numEntries := int(dirent.NameOff) / 12
	if numEntries == 0 {
		return fileMap, nil
	}

	dirents := make([]ErofsDirent, numEntries)
	for i := range numEntries {
		err = readStruct(buf, uint64(i*12), &dirent, "dirent", emptyMagic)
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

//goland:noinspection GoUnhandledErrorResult
func findAndReadCentralDirectory(url string, partitionFilename string, filePath string, list bool, sofOffset uint64, eofOffset uint64) error {
	var eocdHeader EocdRecord
	var eocd64locator Zip64EocdLocator
	var eocd64record Zip64EocdRecord

	sizeofEocdHeader := binary.Size(eocdHeader)
	sizeofEocd64locator := binary.Size(eocd64locator)
	sizeofEocd64record := binary.Size(eocd64record)

	client := &http.Client{}

	eocdOffset := eofOffset - uint64(sizeofEocdHeader+sizeofEocd64locator+sizeofEocd64record)
	eocdBuf, err := fetchRange(url, eocdOffset, eofOffset-1, "end of central directory", client)
	if err != nil {
		return err
	}

	eocdIdx := uint64(sizeofEocd64record + sizeofEocd64locator)
	err = readStruct(eocdBuf, eocdIdx, &eocdHeader, "end of central directory", [4]byte{'P', 'K', 0x05, 0x06})
	if err != nil {
		return err
	}

	cdOffset := uint64(eocdHeader.CdOffset)
	cdSize := uint64(eocdHeader.CdSize)

	if eocdHeader.CdOffset == 0xFFFFFFFF {
		eocd64recordIdx := uint64(0)
		err = readStruct(eocdBuf, eocd64recordIdx, &eocd64record, "zip64 end of central directory record", [4]byte{'P', 'K', 0x06, 0x06})
		if err != nil {
			return err
		}

		cdOffset = eocd64record.CdOffset
		cdSize = eocd64record.CdSize
	}

	innerCdOffset := sofOffset + cdOffset
	cdBuf, err := fetchRange(url, innerCdOffset, innerCdOffset+cdSize-1, "central directory", client)
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

		err = readStruct(cdBuf, offset, &cdfHeader, "central directory file header", [4]byte{'P', 'K', 0x01, 0x02})
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
			err = fetchStruct(url, sofOffset+lfhOffset, &lfHeader, "local file header", [4]byte{'P', 'K', 0x03, 0x04}, client)
			if err != nil {
				return err
			}

			n := uint64(lfHeader.FilenameLen)
			m := uint64(lfHeader.ExtraFieldLen)

			lfOffset := sofOffset + lfhOffset + sizeofLfHeader + n + m

			//goland:noinspection GoRedundantElseInIf
			if nestedZipRegex.MatchString(filename) {
				return findAndReadCentralDirectory(url, partitionFilename, filePath, list, lfOffset, lfOffset+compressedSize)
			} else {
				//goland:noinspection GoRedundantElseInIf
				if filePath != "" {
					if cdfHeader.CompressionMethod == 8 {
						return fmt.Errorf("partition: extracting files in compressed images is not currently supported")
					}

					if !path.IsAbs(filePath) {
						return fmt.Errorf("partition: filePath must be an absolute path")
					}

					var sparseStub SparseStub
					var sparseMagic [4]byte
					binary.LittleEndian.PutUint32(sparseMagic[:], 0xed26ff3a)
					imageOffset := sofOffset + lfhOffset + sizeofLfHeader + n + m
					err = fetchStruct(url, imageOffset, &sparseStub, "sparse stub", sparseMagic, client)
					if err == nil {
						return fmt.Errorf("partition: sparse image format is not currently supported")
					}

					var erofsStub ErofsStub
					var erofsMagic [4]byte
					binary.LittleEndian.PutUint32(erofsMagic[:], 0xe0f5e1e2)
					erofsOffset := imageOffset + 0x400
					err = fetchStruct(url, erofsOffset, &erofsStub, "erofs stub", erofsMagic, client)

					//goland:noinspection GoRedundantElseInIf
					if err == nil {
						var superblock ErofsSuperblock
						err = fetchStruct(url, erofsOffset, &superblock, "erofs superblock", erofsMagic, client)
						if err != nil {
							return err
						}

						blockSize := uint64(1) << superblock.BlkSizeBits
						nidOffset := uint64(superblock.MetaBlkAddr)*blockSize + uint64(superblock.RootNid)*32

						var formatBits uint16
						buf, err := fetchRange(url, imageOffset+nidOffset, imageOffset+nidOffset+1, "inode format check", client)
						if err != nil {
							return err
						}
						formatBits = binary.LittleEndian.Uint16(buf)
						inodeFormat := (formatBits >> 1) & 0x07
						dataMappingType := (formatBits >> 4) & 0x07

						// TODO: if the root inode is a specific format, does that imply the same for all of them?
						if inodeFormat != 2 || dataMappingType != 0 {
							return fmt.Errorf("erofs: node format is not currently supported: %d, %d", inodeFormat, dataMappingType)
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

						return fetchFileErofs(url, imageOffset, uint64(superblock.RootNid), filePathParts, 0, superblock, client)
					} else {
						return fmt.Errorf("ext4 format is not currently supported")
					}
				} else {
					hash, err := fetchFileZip(url, lfOffset, lfOffset+compressedSize-1, uncompressedSize, partitionFilename, cdfHeader.CompressionMethod, client)
					if err != nil {
						return err
					}
					if hash != cdfHeader.CRC32 {
						os.Remove(partitionFilename)
						return fmt.Errorf("central directory: hash mismatch")
					} else {
						return nil
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

//goland:noinspection GoUnhandledErrorResult
func main() {
	var version bool
	flag.BoolVar(&version, "v", false, "")
	flag.BoolVar(&version, "version", false, "")

	var list bool
	flag.BoolVar(&list, "l", false, "")
	flag.BoolVar(&list, "list", false, "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <factoryImageURL> [partitionFilename [filePath]]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Arguments:")
		fmt.Fprintln(os.Stderr, "  factoryImageURL    URL of the factory image")
		fmt.Fprintln(os.Stderr, "  partitionFilename  Name of the partition to download (optional)")
		fmt.Fprintln(os.Stderr, "  filePath           Path to file in partition to download (optional)")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		fmt.Fprintln(os.Stderr, "  -v, --version      Print version and exit")
		fmt.Fprintln(os.Stderr, "  -l, --list         List filenames without downloading")
	}

	flag.Parse()
	args := flag.Args()

	if version {
		fmt.Fprintf(os.Stderr, "pixel-plucker %s\n", Version)
		os.Exit(0)
	}

	if len(args) < 1 && len(args) > 3 {
		flag.Usage()
		os.Exit(1)
	}
	if !list && len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: partitionFilename is required when list flag is not provided")
		flag.Usage()
		os.Exit(1)
	}
	if list && len(args) > 1 {
		fmt.Fprintln(os.Stderr, "Error: listing of files in partitionFilename is not supported")
		flag.Usage()
		os.Exit(1)
	}

	url := args[0]
	partitionFilename := ""
	filePath := ""
	if len(args) > 1 {
		partitionFilename = args[1]
	}
	if len(args) > 2 {
		filePath = args[2]
	}

	res, err := http.Head(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: factory image: HEAD request failed: %v\n", err)
		os.Exit(1)
	}
	res.Body.Close()

	contentLengthStr := res.Header.Get("Content-Length")
	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: factory image: failed to parse Content-Length: %v\n", err)
		os.Exit(1)
	}

	err = findAndReadCentralDirectory(url, partitionFilename, filePath, list, 0, uint64(contentLength))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
