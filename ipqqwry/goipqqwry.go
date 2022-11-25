// ip库更新 https://github.com/wisdomfusion/qqwry.dat
// ip库更新 https://www.cz88.net/ 然后在程序的安装目录找到qqwry.dat
// ip库算法解析包提供者 https://github.com/freshcn/qqwry

package ipqqwry

import (
	"encoding/binary"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const (
	// IndexLen 索引长度
	IndexLen = 7
	// RedirectMode1 国家的类型, 指向另一个指向
	RedirectMode1 = 0x01
	// RedirectMode2 国家的类型, 指向一个指向
	RedirectMode2 = 0x02
)

// ResultQQwry 归属地信息
type ResultQQwry struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	Area    string `json:"area"`
}

type fileData struct {
	Data     []byte
	FilePath string
	Path     *os.File
	IPNum    int64
}

// QQwry 纯真ip库
type QQwry struct {
	Data   *fileData
	Offset int64
}

// Response 向客户端返回数据的
type Response struct {
	r *http.Request
	w http.ResponseWriter
}

// IPData IP库的数据
var IPData fileData

func startInit() {
	demo_path, _ := os.Getwd()
	datFile := flag.String("qqwry", demo_path+"/ipqqwry/data/ipqqwry.dat", "纯真 IP 库的地址")

	IPData.FilePath = *datFile
	res := IPData.InitIPData()

	if v, ok := res.(error); ok {
		log.Println(v)
	}
}

// 查询的IP地址
func FindIPAll(ip string) map[string]ResultQQwry {
	ips := strings.Split(ip, ",")
	qqWry := NewQQwry()
	rs := map[string]ResultQQwry{}
	if len(ips) > 0 {
		for _, v := range ips {
			rs[v] = qqWry.Find(v)
		}
	}
	return rs
}

// 获取IP所在国家
func IpAddr(ip string) string {
	qqWry := NewQQwry()
	x := qqWry.Find(ip)
	return x.Country
}

// InitIPData 初始化ip库数据到内存中
func (f *fileData) InitIPData() (rs interface{}) {
	var tmpData []byte

	// 判断文件是否存在
	_, err := os.Stat(f.FilePath)
	if err != nil && os.IsNotExist(err) {
		log.Println("纯真IP库文件不存在")
	} else {
		// 打开文件句柄
		f.Path, err = os.OpenFile(f.FilePath, os.O_RDONLY, 0400)
		if err != nil {
			rs = err
			return
		}
		defer f.Path.Close()

		tmpData, err = ioutil.ReadAll(f.Path)
		if err != nil {
			log.Println(err)
			rs = err
			return
		}
	}

	f.Data = tmpData

	buf := f.Data[0:8]
	start := binary.LittleEndian.Uint32(buf[:4])
	end := binary.LittleEndian.Uint32(buf[4:])

	f.IPNum = int64((end-start)/IndexLen + 1)

	return true
}

// NewQQwry 新建 qqwry  类型
func NewQQwry() QQwry {
	if IPData.IPNum < 1 {
		startInit()
	}
	return QQwry{
		Data: &IPData,
	}
}

// ReadData 从文件中读取数据
func (q *QQwry) ReadData(num int, offset ...int64) (rs []byte) {
	if len(offset) > 0 {
		q.SetOffset(offset[0])
	}
	nums := int64(num)
	end := q.Offset + nums
	dataNum := int64(len(q.Data.Data))
	if q.Offset > dataNum {
		return nil
	}

	if end > dataNum {
		end = dataNum
	}
	rs = q.Data.Data[q.Offset:end]
	q.Offset = end
	return
}

// SetOffset 设置偏移量
func (q *QQwry) SetOffset(offset int64) {
	q.Offset = offset
}

// Find ip地址查询对应归属地信息
func (q *QQwry) Find(ip string) (res ResultQQwry) {

	res = ResultQQwry{}

	res.IP = ip
	if strings.Count(ip, ".") != 3 {
		return res
	}
	offset := q.searchIndex(binary.BigEndian.Uint32(net.ParseIP(ip).To4()))
	if offset <= 0 {
		return
	}

	var country []byte
	var area []byte

	mode := q.readMode(offset + 4)
	if mode == RedirectMode1 {
		countryOffset := q.readUInt24()
		mode = q.readMode(countryOffset)
		if mode == RedirectMode2 {
			c := q.readUInt24()
			country = q.readString(c)
			countryOffset += 4
		} else {
			country = q.readString(countryOffset)
			countryOffset += uint32(len(country) + 1)
		}
		area = q.readArea(countryOffset)
	} else if mode == RedirectMode2 {
		countryOffset := q.readUInt24()
		country = q.readString(countryOffset)
		area = q.readArea(offset + 8)
	} else {
		country = q.readString(offset + 4)
		area = q.readArea(offset + uint32(5+len(country)))
	}

	enc := simplifiedchinese.GBK.NewDecoder()
	res.Country, _ = enc.String(string(country))
	areas, _ := enc.String(string(area))
	if areas != "CZ88.NET" && areas != " CZ88.NET" {
		res.Area = areas
	} else {
		res.Area = "未知"
	}

	return
}

// readMode 获取偏移值类型
func (q *QQwry) readMode(offset uint32) byte {
	mode := q.ReadData(1, int64(offset))
	return mode[0]
}

// readArea 读取区域
func (q *QQwry) readArea(offset uint32) []byte {
	mode := q.readMode(offset)
	if mode == RedirectMode1 || mode == RedirectMode2 {
		areaOffset := q.readUInt24()
		if areaOffset == 0 {
			return []byte("")
		}
		return q.readString(areaOffset)
	}
	return q.readString(offset)
}

// readString 获取字符串
func (q *QQwry) readString(offset uint32) []byte {
	q.SetOffset(int64(offset))
	data := make([]byte, 0, 30)
	buf := make([]byte, 1)
	for {
		buf = q.ReadData(1)
		if buf[0] == 0 {
			break
		}
		data = append(data, buf[0])
	}
	return data
}

// searchIndex 查找索引位置
func (q *QQwry) searchIndex(ip uint32) uint32 {
	header := q.ReadData(8, 0)

	start := binary.LittleEndian.Uint32(header[:4])
	end := binary.LittleEndian.Uint32(header[4:])

	buf := make([]byte, IndexLen)
	mid := uint32(0)
	_ip := uint32(0)

	for {
		mid = q.getMiddleOffset(start, end)
		buf = q.ReadData(IndexLen, int64(mid))
		_ip = binary.LittleEndian.Uint32(buf[:4])

		if end-start == IndexLen {
			offset := byteToUInt32(buf[4:])
			buf = q.ReadData(IndexLen)
			if ip < binary.LittleEndian.Uint32(buf[:4]) {
				return offset
			}
			return 0
		}

		// 找到的比较大，向前移
		if _ip > ip {
			end = mid
		} else if _ip < ip { // 找到的比较小，向后移
			start = mid
		} else if _ip == ip {
			return byteToUInt32(buf[4:])
		}
	}
}

// readUInt24
func (q *QQwry) readUInt24() uint32 {
	buf := q.ReadData(3)
	return byteToUInt32(buf)
}

// getMiddleOffset
func (q *QQwry) getMiddleOffset(start uint32, end uint32) uint32 {
	records := ((end - start) / IndexLen) >> 1
	return start + records*IndexLen
}

// byteToUInt32 将 byte 转换为uint32
func byteToUInt32(data []byte) uint32 {
	i := uint32(data[0]) & 0xff
	i |= (uint32(data[1]) << 8) & 0xff00
	i |= (uint32(data[2]) << 16) & 0xff0000
	return i
}
