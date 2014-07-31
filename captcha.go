// Copyright 2014 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Package captcha a middleware that provides captcha service for Macaron.
package captcha

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/cache"
)

var (
	defaultChars = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
)

// Captcha represents a captcha service.
type Captcha struct {
	store            cache.Cache
	URLPrefix        string
	FieldIdName      string
	FieldCaptchaName string
	StdWidth         int
	StdHeight        int
	ChallengeNums    int
	Expiration       int64
	CachePrefix      string
}

// generate key string
func (c *Captcha) key(id string) string {
	return c.CachePrefix + id
}

// generate rand chars with default chars
func (c *Captcha) genRandChars() []byte {
	return com.RandomCreateBytes(c.ChallengeNums, defaultChars...)
}

// tempalte func for output html
func (c *Captcha) CreateHtml() template.HTML {
	value, err := c.CreateCaptcha()
	if err != nil {
		panic(fmt.Errorf("fail to create captcha: %v", err))
	}
	return template.HTML(fmt.Sprintf(`<input type="hidden" name="%s" value="%s">
	<a class="captcha" href="javascript:">
		<img onclick="this.src=('%s%s.png?reload='+(new Date()).getTime())" class="captcha-img" src="%s%s.png">
	</a>`, c.FieldIdName, value, c.URLPrefix, value, c.URLPrefix, value))
}

// create a new captcha id
func (c *Captcha) CreateCaptcha() (string, error) {
	id := string(com.RandomCreateBytes(15))
	if err := c.store.Put(c.key(id), c.genRandChars(), c.Expiration); err != nil {
		return "", err
	}
	return id, nil
}

// verify from a request
func (c *Captcha) VerifyReq(req *http.Request) bool {
	req.ParseForm()
	return c.Verify(req.Form.Get(c.FieldIdName), req.Form.Get(c.FieldCaptchaName))
}

// direct verify id and challenge string
func (c *Captcha) Verify(id string, challenge string) (success bool) {
	if len(challenge) == 0 || len(id) == 0 {
		return
	}

	var chars []byte

	key := c.key(id)

	if v, ok := c.store.Get(key).([]byte); ok && len(v) == len(challenge) {
		chars = v
	} else {
		return
	}

	defer c.store.Delete(key)

	// verify challenge
	for i, c := range chars {
		if c != challenge[i]-48 {
			return
		}
	}

	return true
}

type Options struct {
	// URL prefix of getting captcha pictures. Default is "/captcha/".
	URLPrefix string
	// Hidden input element ID. Default is "captcha_id".
	FieldIdName string
	// User input value element name in request form. Default is "captcha".
	FieldCaptchaName string
	// Challenge number. Default is 6.
	ChallengeNums int
	// Captcha expiration time in seconds. Default is 600.
	Expiration int64
	// Cache key prefix captcha characters. Default is "captcha_".
	CachePrefix string
}

func prepareOptions(options []Options) Options {
	var opt Options
	if len(options) > 0 {
		opt = options[0]
	}

	// Defaults.
	if len(opt.URLPrefix) == 0 {
		opt.URLPrefix = "/captcha/"
	} else if opt.URLPrefix[len(opt.URLPrefix)-1] != '/' {
		opt.URLPrefix += "/"
	}

	if len(opt.FieldIdName) == 0 {
		opt.FieldIdName = "captcha_id"
	}

	if len(opt.FieldCaptchaName) == 0 {
		opt.FieldCaptchaName = "captcha"
	}

	if opt.ChallengeNums == 0 {
		opt.ChallengeNums = 6
	}

	if opt.Expiration == 0 {
		opt.Expiration = 600
	}

	if len(opt.CachePrefix) == 0 {
		opt.CachePrefix = "captcha_"
	}

	return opt
}

// NewCaptcha initializes and returns a captcha with given options.
func NewCaptcha(opt Options) *Captcha {
	return &Captcha{
		URLPrefix:        opt.URLPrefix,
		FieldIdName:      opt.FieldIdName,
		FieldCaptchaName: opt.FieldCaptchaName,
		StdWidth:         stdWidth,
		StdHeight:        stdHeight,
		ChallengeNums:    opt.ChallengeNums,
		Expiration:       opt.Expiration,
		CachePrefix:      opt.CachePrefix,
	}
}

// Captchaer is a middleware that maps a captcha.Captcha service into the Macaron handler chain.
// An single variadic captcha.Options struct can be optionally provided to configure.
// This should be register after cache.Cacher.
func Captchaer(options ...Options) macaron.Handler {
	cpt := NewCaptcha(prepareOptions(options))
	return func(ctx *macaron.Context, cache cache.Cache) {
		cpt.store = cache

		if strings.HasPrefix(ctx.Req.RequestURI, cpt.URLPrefix) {
			var chars []byte
			id := path.Base(ctx.Req.RequestURI)
			if i := strings.Index(id, "."); i > -1 {
				id = id[:i]
			}
			key := cpt.key(id)

			// Reload captcha.
			if len(ctx.Query("reload")) > 0 {
				chars = cpt.genRandChars()
				if err := cpt.store.Put(key, chars, cpt.Expiration); err != nil {
					ctx.Status(500)
					ctx.Write([]byte("captcha reload error"))
					panic(fmt.Errorf("fail to reload captcha: %v", err))
				}
			} else {
				if v, ok := cpt.store.Get(key).([]byte); ok {
					chars = v
				} else {
					ctx.Status(404)
					ctx.Write([]byte("captcha not found"))
					return
				}
			}

			if _, err := NewImage(chars, cpt.StdWidth, cpt.StdHeight).WriteTo(ctx.Resp); err != nil {
				panic(fmt.Errorf("fail to write captcha: %v", err))
			}
			return
		}

		ctx.Data["Captcha"] = cpt
		ctx.Map(cpt)
	}
}
