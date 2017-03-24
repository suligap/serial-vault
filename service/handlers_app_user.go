// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package service

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"time"

	"fmt"

	"github.com/snapcore/snapd/asserts"
)

var userIndexTemplate = "/static/app_user.html"

// SystemUserRequest is the JSON version of the request to create a system-user assertion
type SystemUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	ModelID  int    `json:"model"`
	Since    string `json:"since"`
}

// SystemUserResponse is the response from a system-user creation
type SystemUserResponse struct {
	Success      bool   `json:"success"`
	ErrorCode    string `json:"error_code"`
	ErrorSubcode string `json:"error_subcode"`
	ErrorMessage string `json:"message"`
	Assertion    string `json:"assertion"`
}

// UserIndexHandler is the front page of the web application
func UserIndexHandler(w http.ResponseWriter, r *http.Request) {
	page := Page{Title: Environ.Config.Title, Logo: Environ.Config.Logo}

	path := []string{Environ.Config.DocRoot, userIndexTemplate}
	t, err := template.ParseFiles(strings.Join(path, ""))
	if err != nil {
		log.Printf("Error loading the application template: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SystemUserAssertionHandler is the API method to generate a signed system-user assertion for a device
func SystemUserAssertionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Decode the body
	user := SystemUserRequest{}
	err := json.NewDecoder(r.Body).Decode(&user)
	switch {
	// Check we have some data
	case err == io.EOF:
		w.WriteHeader(http.StatusBadRequest)
		formatBooleanResponse(false, "error-user-data", "", "No system-user data supplied", w)
		return
		// Check for parsing errors
	case err != nil:
		w.WriteHeader(http.StatusBadRequest)
		formatBooleanResponse(false, "error-decode-json", "", err.Error(), w)
		return
	}

	// Get the model
	model, err := Environ.DB.GetModel(user.ModelID)
	if err != nil {
		logMessage("USER", "invalid-model", "Cannot find model with the selected ID")
		formatBooleanResponse(false, "invalid-model", "", "Cannot find model with the selected ID", w)
		return
	}

	// Check that the model has an active keypair
	if !model.KeyActive {
		logMessage("USER", "invalid-model", "The model is linked with an inactive signing-key")
		formatBooleanResponse(false, "invalid-model", "", "The model is linked with an inactive signing-key", w)
		return
	}

	// Create the system-user assertion headers from the request
	assertionHeaders := userRequestToAssertion(user, model)

	// Sign the system-user assertion
	signedAssertion, err := Environ.KeypairDB.SignAssertion(asserts.SystemUserType, assertionHeaders, nil, model.AuthorityID, model.KeyID, model.SealedKey)
	if err != nil {
		logMessage("USER", "signing-assertion", err.Error())
		formatBooleanResponse(false, "signing-assertion", "", err.Error(), w)
		return
	}

	// Return the signed assertion
	serializedAssertion := asserts.Encode(signedAssertion)

	response := SystemUserResponse{Success: true, Assertion: string(serializedAssertion)}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logMessage("USER", "signing-assertion", err.Error())
	}
}

func userRequestToAssertion(user SystemUserRequest, model Model) map[string]interface{} {

	// Create the salt from the keystore secret
	reg, _ := regexp.Compile("[^A-Za-z0-9]+")
	baseSalt := reg.ReplaceAllString(Environ.Config.KeyStoreSecret, "")

	// Encrypt the password
	salt := fmt.Sprintf("$6$%s$", baseSalt)
	password := cryptUser(user.Password, salt)

	// Set the since and end date/times
	since, err := time.Parse("YYYY-MM-DDThh:mm:ssZ00:00", user.Since)
	if err != nil {
		since = time.Now().UTC()
	}
	until := since.Add(time.Duration(24*365) * time.Hour)

	// Create the serial assertion header from the serial-request headers
	headers := map[string]interface{}{
		"type":              asserts.SystemUserType.Name,
		"revision":          "1",
		"authority-id":      model.AuthorityID,
		"brand-id":          model.AuthorityID,
		"email":             user.Email,
		"name":              user.Name,
		"username":          user.Username,
		"password":          password,
		"models":            []interface{}{model.Name},
		"since":             since.Format(time.RFC3339),
		"until":             until.Format(time.RFC3339),
		"sign-key-sha3-384": model.KeyID,
	}

	// Create a new serial assertion
	return headers
}
