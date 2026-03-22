// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// Text is a Section that renders a free-form text block.
type Text struct {
	SectionName string `json:"name"`
	Body        string `json:"body"`
}

// Type returns the discriminator "text".
func (txt *Text) Type() string { return "text" }

// Name returns the human-readable section heading.
func (txt *Text) Name() string { return txt.SectionName }

// Render writes the text block in the requested format.
func (txt *Text) Render(format Format, writer io.Writer) error {
	switch format {
	case FormatText:
		_, err := fmt.Fprint(writer, txt.Body)
		return err
	case FormatJSON:
		return txt.renderJSON(writer)
	default:
		return fmt.Errorf("unsupported format %q for text section", format)
	}
}

func (txt *Text) renderJSON(writer io.Writer) error {
	envelope := struct {
		Type string `json:"type"`
		Name string `json:"name"`
		Body string `json:"body"`
	}{
		Type: txt.Type(),
		Name: txt.SectionName,
		Body: txt.Body,
	}

	encoder := json.NewEncoder(writer)

	return encoder.Encode(envelope)
}
