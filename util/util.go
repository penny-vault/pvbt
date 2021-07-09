package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Struct used for sorting tickers by a float64 value (i.e. momentum)
type Pair struct {
	Key   string
	Value float64
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }

// DoEvery execute function every d duration
func DoEvery(d time.Duration, f func()) {
	f()
	for range time.Tick(d) {
		f()
	}
}

// ArrToUpper uppercase every string in array
func ArrToUpper(arr []string) {
	for ii := range arr {
		arr[ii] = strings.ToUpper(arr[ii])
	}
}

// Encrypt an array of byte data using the PV_SECRET key
func Encrypt(data []byte) ([]byte, error) {
	key, err := hex.DecodeString(os.Getenv("PV_SECRET"))
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("could not unhexlify PV_SECRET")
		return nil, err
	}

	block, _ := aes.NewCipher(key)
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("could not create cipher")
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("could not create nonce")
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Decrypt data and return using PV_SECRET key
func Decrypt(data []byte) ([]byte, error) {
	key, err := hex.DecodeString(os.Getenv("PV_SECRET"))
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("could not unhexlify PV_SECRET")
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("could not create cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not unencrypt data")
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not read unencryptd data")
		return nil, err
	}
	return plaintext, nil
}
