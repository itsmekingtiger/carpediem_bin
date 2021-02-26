package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	viper.SetConfigFile("update.toml")
	viper.SetConfigType("toml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("failed to load config: ", err)
	}

	var jack = &lumberjack.Logger{
		Filename:   viper.GetString("log-file"),
		MaxSize:    1,
		MaxBackups: 30,
		MaxAge:     180,
		LocalTime:  true,
	}
	log.SetReportCaller(true)
	log.SetOutput(io.MultiWriter(os.Stdout, jack))
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{ForceColors: true})

	endpoint = viper.GetString("endpoint")
	execName = viper.GetString("exec")
	execPath = viper.GetString("execpath")
}

var (
	endpoint string
	execName string
	execPath string
)

func main() {
	log.Info("===starting update service===")

	var _delay = make([]byte, 1)
	_, err := rand.Read(_delay)
	if err != nil {
		log.Error("failed to read rand delay: ", err)
		return
	}
	delay := time.Duration(_delay[0] % 60)
	log.Infof("delay %d second", delay)
	time.Sleep(delay * time.Second)
	log.Info("delay finished")

	log.Debug("다음 위치에 업데이트: ", path.Join(execPath, execName))

	_execRes, err := exec.Command(
		"md5sum",
		path.Join(execPath, execName),
	).Output()

	if err != nil {
		log.Fatal("failed to execute child process:", err)
	}

	execRes := strings.Split(string(_execRes), " ")
	log.Infof("current binary's hash: %s", execRes[0])

	// 요청
	log.Infof("patching update information from: %s", endpoint)
	resp, err := http.Get(endpoint)
	if err != nil {
		log.Fatalf("failed to patching update information: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("response code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read body: %s", err.Error())
	}

	// 결과 언마셜
	var updateInfo UpdateInfo
	if err := json.Unmarshal(data, &updateInfo); err != nil {
		log.Fatalf("failed to unmarshal data: %s", err.Error())
	}

	if updateInfo.MD5 == execRes[0] {
		log.Infof("binary is up-to-date(%s)", updateInfo.MD5)
		return
	}
	log.Infof("binary is :%s", updateInfo.MD5)

	// 다운로드
	tmpLoc := path.Join("/tmp", execName)
	if err := download(tmpLoc, updateInfo.Link); err != nil {
		log.Fatal(err)
	}

	newMD5, err := hash_file_md5(tmpLoc)
	if err != nil {
		log.Fatal("")
	}

	if newMD5 != updateInfo.MD5 {
		log.Fatalf("해쉬 일치 하지 않음(calc: %s, fromJson: %s)", newMD5, updateInfo.MD5)
	}

	fInfo, err := os.Stat(tmpLoc)
	if err != nil {
		log.Fatal("failed to load file information: ", err)
	}
	log.Infof("%+ v", fInfo)
	// return

	os.Rename(tmpLoc, path.Join(execPath, execName))
	log.Info("complete to download")

	if err := os.Chmod(path.Join(execPath, execName), 0755); err != nil {
		log.Fatal("failed to change mode: ", err)
	}

	// kill
	url := "http://localhost:55001/api/service/shutdown"

	resp, err = http.Post(url, "text/plain", bytes.NewBufferString("ack"))
	if err != nil {
		log.Fatal("failed to restart service: ", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatal("failed to restart service: status is ", resp.StatusCode)
	}
	log.Info("done")
}

func download(filepath string, url string) error {

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// UpdateInfo -
type UpdateInfo struct {
	MD5  string `json:"md5"`
	Link string `json:"link"`
}

func hash_file_md5(filePath string) (string, error) {
	//Initialize variable returnMD5String now in case an error has to be returned
	var returnMD5String string

	//Open the passed argument and check for any error
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}

	//Tell the program to call the following function when the current function returns
	defer file.Close()

	//Open a new hash interface to write to
	hash := md5.New()

	//Copy the file in the hash interface and check for any error
	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}

	//Get the 16 bytes hash
	hashInBytes := hash.Sum(nil)[:16]

	//Convert the bytes to a string
	returnMD5String = hex.EncodeToString(hashInBytes)

	return returnMD5String, nil

}
