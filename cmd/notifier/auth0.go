package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/goccy/go-json"

	log "github.com/sirupsen/logrus"
)

type Token struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type Auth0User struct {
	UserID        string                 `json:"user_id"`
	Name          string                 `json:"name"`
	Email         string                 `json:"email"`
	EmailVerified bool                   `json:"email_verified"`
	UserMetaData  map[string]interface{} `json:"user_metadata"`
}

type User struct {
	ID          string
	Name        string
	Email       string
	Verified    bool
	TiingoToken string
}

var userMap map[string]User = make(map[string]User)

func getToken() (string, error) {
	domain := os.Getenv("AUTH0_DOMAIN")
	clientID := os.Getenv("AUTH0_CLIENT_ID")
	secret := os.Getenv("AUTH0_SECRET")

	url := fmt.Sprintf("%s/oauth/token", domain)
	bodyStr := fmt.Sprintf(`grant_type=client_credentials&client_id=%s&client_secret=%s&audience=%s/api/v2/`, clientID, secret, domain)
	body := strings.NewReader(bodyStr)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Domain":   domain,
				"ClientId": clientID,
				"Error":    err,
			}).Error("Cannot build Auth0 Management API access token request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Domain":   domain,
				"ClientId": clientID,
				"Error":    err,
			}).Error("cannot get Auth0 Management API access token request")
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// NOTE: It's save to ignore the err return here
		// as we are just formatting an error message for the
		// already errored out HTTP response
		respBody, _ := ioutil.ReadAll(resp.Body)
		msg := "cannot get Auth0 Management API access token request"
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error(msg)
		return "", errors.New(msg)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := "failed to read body response when retrieving Auth0 Management API access token"
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error(msg)
		return "", errors.New(msg)
	}

	managementToken := &Token{}
	err = json.Unmarshal(respBody, managementToken)
	if err != nil {
		msg := "failed to convert json Auth0 Management API access token"
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error(msg)
		return "", errors.New(msg)
	}

	return managementToken.AccessToken, nil
}

func getUser(userID string) (*User, error) {
	// Check if user is already in cache
	if u, ok := userMap[userID]; ok {
		return &u, nil
	}

	log.WithFields(log.Fields{
		"UserId": userID,
	}).Info("requesting user from auth0")

	// User hasn't been loaded yet, request from identity provider
	domain := os.Getenv("AUTH0_DOMAIN")
	token, err := getToken()
	if err != nil {
		return nil, err
	}

	encodedUserID := url.QueryEscape(userID)
	url := fmt.Sprintf("%s/api/v2/users/%s", domain, encodedUserID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
		}).Error("could not create Auth0 user request")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
		}).Error("user account request failed")
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		msg := "user account request failed"
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
			"Body":   string(respBody),
		}).Error(msg)
		return nil, errors.New(msg)
	}

	respBody, _ := ioutil.ReadAll(resp.Body)
	auth0User := Auth0User{}
	err = json.Unmarshal(respBody, &auth0User)
	if err != nil {
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
			"Body":   string(respBody),
		}).Error("could not decode user response")
	}

	u := User{
		ID:       userID,
		Name:     auth0User.Name,
		Email:    auth0User.Email,
		Verified: auth0User.EmailVerified,
	}

	if v, ok := auth0User.UserMetaData["tiingo_token"]; ok {
		msg := "could not decode user response - tiingo token invalid type"
		if tiingoToken, ok := v.(string); ok {
			u.TiingoToken = tiingoToken
		} else {
			log.WithFields(log.Fields{
				"Domain": domain,
				"UserId": userID,
				"Error":  err,
				"Body":   string(respBody),
			}).Error(msg)
			return nil, errors.New(msg)
		}
	} else {
		msg := "could not decode user response - tiingo token missing"
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
			"Body":   string(respBody),
		}).Error(msg)
		return nil, errors.New(msg)
	}

	userMap[userID] = u

	return &u, nil
}
