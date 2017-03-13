package integrationtests

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"strconv"

	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/testdata"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

const (
	dataLen     = 500 * 1024       // 500 KB
	dataLongLen = 50 * 1024 * 1024 // 50 MB
)

var (
	server     *h2quic.Server
	dataMan    dataManager
	port       string
	uploadDir  string
	clientPath string
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Tests Suite")
}

var _ = BeforeSuite(func() {
	setupHTTPHandlers()
	setupQuicServer()
})

var _ = AfterSuite(func() {
	err := server.Close()
	Expect(err).NotTo(HaveOccurred())
}, 10)

var _ = BeforeEach(func() {
	// create a new uploadDir for every test
	var err error
	uploadDir, err = ioutil.TempDir("", "quic-upload-dest")
	Expect(err).ToNot(HaveOccurred())
	err = os.MkdirAll(uploadDir, os.ModeDir|0777)
	Expect(err).ToNot(HaveOccurred())

	_, thisfile, _, ok := runtime.Caller(0)
	if !ok {
		Fail("Failed to get current path")
	}
	clientPath = filepath.Join(thisfile, fmt.Sprintf("../../../quic-clients/client-%s-debug", runtime.GOOS))
})

var _ = AfterEach(func() {
	// remove uploadDir
	if len(uploadDir) < 20 {
		panic("uploadDir too short")
	}
	os.RemoveAll(uploadDir)

	removeDownloadData()
})

func setupHTTPHandlers() {
	defer GinkgoRecover()

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		_, err := io.WriteString(w, "Hello, World!\n")
		Expect(err).NotTo(HaveOccurred())
	})

	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		data := dataMan.GetData()
		Expect(data).ToNot(HaveLen(0))
		_, err := w.Write(data)
		Expect(err).NotTo(HaveOccurred())
	})

	http.HandleFunc("/data/", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		data := dataMan.GetData()
		Expect(data).ToNot(HaveLen(0))
		_, err := w.Write(data)
		Expect(err).NotTo(HaveOccurred())
	})

	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		body, err := ioutil.ReadAll(r.Body)
		Expect(err).NotTo(HaveOccurred())
		_, err = w.Write(body)
		Expect(err).NotTo(HaveOccurred())
	})

	// requires the num GET parameter, e.g. /uploadform?num=2
	// will create num input fields for uploading files
	http.HandleFunc("/uploadform", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		num, err := strconv.Atoi(r.URL.Query().Get("num"))
		Expect(err).ToNot(HaveOccurred())
		response := "<html><body>\n<form id='form' action='https://quic.clemente.io/uploadhandler' method='post' enctype='multipart/form-data'>"
		for i := 0; i < num; i++ {
			response += "<input type='file' id='upload_" + strconv.Itoa(i) + "' name='uploadfile_" + strconv.Itoa(i) + "' />"
		}
		response += "</form><body></html>"
		_, err = io.WriteString(w, response)
		Expect(err).NotTo(HaveOccurred())
	})

	http.HandleFunc("/uploadhandler", func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()

		err := r.ParseMultipartForm(100 * (1 << 20)) // max. 100 MB
		Expect(err).ToNot(HaveOccurred())

		count := 0
		for {
			var file multipart.File
			var handler *multipart.FileHeader
			file, handler, err = r.FormFile("uploadfile_" + strconv.Itoa(count))
			if err != nil {
				break
			}
			count++
			f, err2 := os.OpenFile(path.Join(uploadDir, handler.Filename), os.O_WRONLY|os.O_CREATE, 0666)
			Expect(err2).ToNot(HaveOccurred())
			io.Copy(f, file)
			f.Close()
			file.Close()
		}
		Expect(count).ToNot(BeZero()) // there have been at least one uploaded file in this request

		_, err = io.WriteString(w, "")
		Expect(err).NotTo(HaveOccurred())
	})
}

func setupQuicServer() {
	server = &h2quic.Server{
		Server: &http.Server{
			TLSConfig: testdata.GetTLSConfig(),
		},
	}

	addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred())
	conn, err := net.ListenUDP("udp", addr)
	Expect(err).NotTo(HaveOccurred())
	port = strconv.Itoa(conn.LocalAddr().(*net.UDPAddr).Port)

	go func() {
		defer GinkgoRecover()
		server.Serve(conn)
	}()
}

// getDownloadSize gets the file size of a file in the local download folder
func getDownloadSize(filename string) int {
	stat, err := os.Stat("/Users/lucas/Downloads/" + filename)
	if err != nil {
		return 0
	}
	return int(stat.Size())
}

// getDownloadMD5 gets the md5 sum file of a file in the local download folder
func getDownloadMD5(filename string) []byte {
	var result []byte
	file, err := os.Open("/Users/lucas/Downloads/" + filename)
	if err != nil {
		return nil
	}
	defer file.Close()

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return nil
	}
	return hash.Sum(result)
}

func removeDownloadData() {
	// TODO(lclemente)
}
