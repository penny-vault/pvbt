// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

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
	"github.com/spf13/viper"
)

const (
	TickerName = "TICKER"
	DateIdx    = "DATE"
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
	key, err := hex.DecodeString(viper.GetString("secret_key"))
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
	key, err := hex.DecodeString(viper.GetString("secret_key"))
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

func SetupLogging() {
	// Set level
	level := viper.GetString("log.level")
	level = strings.ToLower(level)

	switch level {
	case "debug":
		log.Info("Setting logging level to debug")
		log.SetLevel(log.DebugLevel)
	case "error":
		log.Info("Setting logging level to error")
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.Info("Setting logging level to fatal")
		log.SetLevel(log.FatalLevel)
	case "info":
		log.Info("Setting logging level to info")
		log.SetLevel(log.InfoLevel)
	case "panic":
		log.Info("Setting logging level to panic")
		log.SetLevel(log.PanicLevel)
	case "trace":
		log.Info("Setting logging level to trace")
		log.SetLevel(log.TraceLevel)
	case "warning":
		log.Info("Setting logging level to warning")
		log.SetLevel(log.WarnLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}

	// Set report caller
	log.SetReportCaller(viper.GetBool("log.report_caller"))

	// Setup output
	output := viper.GetString("log.output")
	switch output {
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stderr)
	default:
		fh, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		defer fh.Close()
		log.SetOutput(fh)
	}
}
