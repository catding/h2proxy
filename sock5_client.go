package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"golang.org/x/net/http2"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"

	//"os"
	"errors"
	"os"
)

type targetInfo struct {
	host string
	port string
}

var (
	proxy     string
	local     string
	localHost string
	localPort string
	proxyHost string
	proxyPort string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.StringVar(&localHost, "local_host", "localhost", "-local_host=127.0.0.1")
	flag.StringVar(&localPort, "local_port", "3002", "-local_port=4000")
	flag.StringVar(&proxyHost, "proxy_host", "", "-porxy_host=xxx.xxx.xxx.xxx")
	flag.StringVar(&proxyPort, "proxy_port", "", "-proxy_port=3000")

	flag.Parse()
	if proxyHost == "" {
		flag.Usage()
		log.Println("proxy_host is required")
		os.Exit(1)
	}
	if proxyPort == "" {
		log.Println("proxy_port is required")
		flag.Usage()

		os.Exit(1)
	}
	if localHost == "" {
		log.Println("local_host is required")
		flag.Usage()

		os.Exit(1)
	}
	if localPort == "" {
		log.Println("local_port is requred")
		flag.Usage()

		os.Exit(1)
	}
	proxy = fmt.Sprintf("%s:%s", proxyHost, proxyPort)
	local = fmt.Sprintf("%s:%s", localHost, localPort)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func auth(conn net.Conn) error {
	// VER	NMETHODS	METHODS
	// 1	1			1-255

	// VER是SOCKS版本，这里应该是0x05；
	// NMETHODS是METHODS部分的长度；
	// METHODS是客户端支持的认证方式列表，每个方法占1字节。当前的定义是：
	// 0x00 不需要认证
	// 0x01 GSSAPI
	// 0x02 用户名、密码认证
	// 0x03 - 0x7F由IANA分配（保留）
	// 0x80 - 0xFE为私人方法保留
	// 0xFF 无可接受的方法

	res := make([]byte, 2)
	_, err := conn.Read(res)
	if err != nil {

		log.Println(err)
		return err
	}
	log.Println(res)
	methodLength := res[1]
	method := make([]byte, methodLength)
	conn.Read(method)
	log.Println(method)

	// 服务器从客户端提供的方法中选择一个并通过以下消息通知客户端（以字节为单位）：
	//
	// VER	METHOD
	// 1	1
	// VER是SOCKS版本，这里应该是0x05；
	// METHOD是服务端选中的方法。如果返回0xFF表示没有一个认证方法被选中，客户端需要关闭连接。

	// REP应答字段
	// 0x00表示成功
	// 0x01普通SOCKS服务器连接失败
	// 0x02现有规则不允许连接
	// 0x03网络不可达
	// 0x04主机不可达
	// 0x05连接被拒
	// 0x06 TTL超时
	// 0x07不支持的命令
	// 0x08不支持的地址类型
	// 0x09 - 0xFF未定义
	resp := []byte{5, 0}
	conn.Write(resp)
	return nil
}

func buildDestConn(conn net.Conn) (*targetInfo, error) {
	// VER	CMD	RSV		ATYP	DST.ADDR	DST.PORT
	// 1	1	0x00	1		动态		2
	// VER是SOCKS版本，这里应该是0x05；
	// CMD是SOCK的命令码
	// 0x01表示CONNECT请求
	// 0x02表示BIND请求
	// 0x03表示UDP转发
	// RSV 0x00，保留
	// ATYP DST.ADDR类型
	// 0x01 IPv4地址，DST.ADDR部分4字节长度
	// 0x03 域名，DST.ADDR部分第一个字节为域名长度，DST.ADDR剩余的内容为域名，没有\0结尾。
	// 0x04 IPv6地址，16个字节长度。
	// DST.ADDR 目的地址
	// DST.PORT 网络字节序表示的目的端口

	res := make([]byte, 1024)
	n, err := conn.Read(res)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println(res[:n])

	if res[1] != 1 {
		log.Println("MEHOTDS NOT SUPPORTED")
		resp := []byte{5, 7, 0}
		conn.Write(resp)
		return nil, err
	}

	target := targetInfo{}
	if res[3] == 1 {
		addr := fmt.Sprintf("%d.%d.%d.%d", res[4], res[5], res[6], res[7])
		port := int(res[8])*256 + int(res[9])
		target.port = strconv.Itoa(port)
		target.host = addr

		resp := []byte{5, 0, 0, res[3], res[4], res[5], res[6], res[7], res[8], res[9]}
		conn.Write(resp)
	} else if res[3] == 3 {
		length := int(res[4])
		addr := res[5 : 5+length]
		target.host = string(addr)
		port := int(res[4+length+1])*256 + int(res[4+length+2])
		target.port = strconv.Itoa(port)
		resp := []byte{5, 0, 0, res[3], res[4]}
		resp = append(resp, res[5:n]...)
		conn.Write(resp)
	} else if res[3] == 4 {
		resp := []byte{5, 8, 0}
		conn.Write((resp))
		log.Println("IPV6 Not Implemented")
		return nil, errors.New("ipv6 NotImplemnted")
	}

	// target := fmt.Sprintf("%s:%d", addr, port)

	//服务器按以下格式回应客户端的请求（以字节为单位）：
	//
	//VER	REP	RSV		ATYP	BND.ADDR	BND.PORT
	//1		1	0x00	1		动态			2
	//VER是SOCKS版本，这里应该是0x05；
	//REP应答字段
	//0x00表示成功
	//0x01普通SOCKS服务器连接失败
	//0x02现有规则不允许连接
	//0x03网络不可达
	//0x04主机不可达
	//0x05连接被拒
	//0x06 TTL超时
	//0x07不支持的命令
	//0x08不支持的地址类型
	//0x09 - 0xFF未定义
	//RSV 0x00，保留
	//ATYP BND.ADDR类型
	//0x01 IPv4地址，DST.ADDR部分4字节长度
	//0x03域名，DST.ADDR部分第一个字节为域名长度，DST.ADDR剩余的内容为域名，没有\0结尾。
	//0x04 IPv6地址，16个字节长度。
	//BND.ADDR 服务器绑定的地址
	//BND.PORT 网络字节序表示的服务器绑定的端口

	return &target, nil
}
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	// _, err := io.Copy(destination, source)
	body := make([]byte, 102400)
	n, err := source.Read(body)
	if err != nil {
		log.Println(err)
	}
	log.Println(string(body[:n]))
	destination.Write(body[:n])
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	err := auth(conn)
	if err != nil {
		return
	}
	dest, err := buildDestConn(conn)
	if dest == nil || err != nil {
		return
	}

	remoteAddr := "http://" + dest.host + ":" + dest.port
	ToHttpProxy(conn, proxy, remoteAddr)

	//go transfer(destConn.writer, conn)
	//go transfer(conn, destConn.reader)
}

func ToHttpProxy(from net.Conn, proxy, remoteAddr string) {

	tr := &http2.Transport{
		DialTLS: func(network, addr string, config *tls.Config) (net.Conn, error) {
			return tls.Dial("tcp", proxy, &tls.Config{
				NextProtos:         []string{"h2"},
				InsecureSkipVerify: true,
			})
		},
		AllowHTTP: true,
	}

	r, w := io.Pipe()

	//remoteAddr := "http://216.58.200.14:443"
	log.Println(remoteAddr)
	req, err := http.NewRequest(
		http.MethodConnect,
		remoteAddr,
		r,
	)

	if err != nil {
		log.Println(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Println(resp.StatusCode)
		io.Copy(os.Stdout, resp.Body)
		log.Println("Connect Proxy Server Error")
		return
	}

	go io.Copy(w, from)
	io.Copy(from, resp.Body)
}

func main() {
	ln, err := net.Listen("tcp", localHost+":"+localPort)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConnection(conn)
	}
}
