// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017-2018 Canonical Ltd
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

package usso

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/CanonicalLtd/serial-vault/datastore"
	"github.com/juju/usso"
	"github.com/juju/usso/openid"
)

var (
	teams    = "ce-web-logs,canonical"
	required = "email,fullname,nickname"
	optional = ""
)

var client = openid.NewClient(usso.ProductionUbuntuSSOServer, nil, nil)

// LoginHandler processes the login for Ubuntu SSO
func LoginHandler(w http.ResponseWriter, r *http.Request) {

	r.ParseForm()

	url := *r.URL

	// Set the return URL: from the OpenID login with the full domain name
	url.Scheme = datastore.Environ.Config.URLScheme
	url.Host = datastore.Environ.Config.URLHost

	if r.Form.Get("openid.ns") == "" {
		req := openid.Request{
			ReturnTo:     url.String(),
			Teams:        strings.FieldsFunc(teams, isComma),
			SRegRequired: strings.FieldsFunc(required, isComma),
			SRegOptional: strings.FieldsFunc(optional, isComma),
		}
		url := client.RedirectURL(&req)
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	resp, err := client.Verify(url.String())
	if err != nil {
		w.Header().Set("ContentType", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		errorTemplate.Execute(w, err)
		return
	}

	// TODO: Verify the permissions of the user against the database

	// Build the JWT
	jwtToken, err := NewJWTToken(resp)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errorTemplate.Execute(w, err)
		return
	}

	// Set a cookie with the JWT
	AddJWTCookie(jwtToken, w)

	// Redirect to the homepage with the JWT
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func isComma(c rune) bool {
	return c == ','
}

var errorTemplate = template.Must(template.New("failure").Parse(`<html>
<head><title>Login Error</title></head>
<body>{{.}}</body>
</html>
`))
