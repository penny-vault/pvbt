package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

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
			}).Error("Cannot get Auth0 Management API access token request")
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// NOTE: It's save to ignore the err return here
		// as we are just formatting an error message for the
		// already errored out HTTP response
		respBody, _ := ioutil.ReadAll(resp.Body)
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error("Cannot get Auth0 Management API access token request")
		return "", errors.New("Cannot get Auth0 Management API access token request")
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error("Failed to read body response when retrieving Auth0 Management API access token")
		return "", errors.New("Cannot get Auth0 Management API access token request")
	}

	managementToken := &Token{}
	err = json.Unmarshal(respBody, managementToken)
	if err != nil {
		log.WithFields(
			log.Fields{
				"Domain":     domain,
				"ClientId":   clientID,
				"StatusCode": resp.StatusCode,
				"Headers":    resp.Header,
				"Body":       respBody,
			}).Error("Failed to convert json Auth0 Management API access token")
		return "", errors.New("Cannot get Auth0 Management API access token request")
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
	}).Info("Requesting user from auth0")

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
		}).Error("Could not create Auth0 user request")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
		}).Error("User account request failed")
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
			"Body":   string(respBody),
		}).Error("User account request failed")
		return nil, errors.New("User account request failed")
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
		}).Error("Could not decode user response")
	}

	u := User{
		ID:       userID,
		Name:     auth0User.Name,
		Email:    auth0User.Email,
		Verified: auth0User.EmailVerified,
	}

	if v, ok := auth0User.UserMetaData["tiingo_token"]; ok {
		if tiingoToken, ok := v.(string); ok {
			u.TiingoToken = tiingoToken
		} else {
			log.WithFields(log.Fields{
				"Domain": domain,
				"UserId": userID,
				"Error":  err,
				"Body":   string(respBody),
			}).Error("Could not decode user response - tiingo token invalid type")
			return nil, errors.New("Could not decode user response - tiingo token invalid type")
		}
	} else {
		log.WithFields(log.Fields{
			"Domain": domain,
			"UserId": userID,
			"Error":  err,
			"Body":   string(respBody),
		}).Error("Could not decode user response - tiingo token missing")
		return nil, errors.New("Could not decode user response - tiingo token missing")
	}

	userMap[userID] = u

	return &u, nil
}
