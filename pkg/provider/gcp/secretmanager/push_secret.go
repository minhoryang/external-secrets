/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package secretmanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tidwall/sjson"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	esv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
)

type Metadata struct {
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
}

func newPushSecretBuilder(payload []byte, metadata *apiextensionsv1.JSON, remoteRef esv1beta1.PushRemoteRef) (pushSecretBuilder, error) {
	if remoteRef.GetProperty() == "" {
		return &psBuilder{
			payload:   payload,
			metadata:  metadata,
			remoteRef: remoteRef,
		}, nil
	}

	if metadata != nil {
		return nil, errors.New("cannot specify metadata and property at the same time")
	}

	return &propertyPSBuilder{
		payload:   payload,
		remoteRef: remoteRef,
	}, nil
}

type pushSecretBuilder interface {
	buildMetadata(annotations, labels map[string]string) (map[string]string, map[string]string, error)
	needUpdate(original []byte) bool
	buildData(original []byte) ([]byte, error)
}

type psBuilder struct {
	payload   []byte
	metadata  *apiextensionsv1.JSON
	remoteRef esv1beta1.PushRemoteRef
}

func (b *psBuilder) buildMetadata(_, labels map[string]string) (map[string]string, map[string]string, error) {
	if manager, ok := labels[managedByKey]; !ok || manager != managedByValue {
		return nil, nil, fmt.Errorf("secret %v is not managed by external secrets", b.remoteRef.GetRemoteKey())
	}

	var metadata Metadata
	if b.metadata != nil {
		decoder := json.NewDecoder(bytes.NewReader(b.metadata.Raw))
		// Want to return an error if unknown fields exist
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&metadata); err != nil {
			return nil, nil, fmt.Errorf("failed to decode PushSecret metadata: %w", err)
		}
	}

	newLabels := map[string]string{}
	if metadata.Labels != nil {
		newLabels = metadata.Labels
	}
	newLabels[managedByKey] = managedByValue

	return metadata.Annotations, newLabels, nil
}

func (b *psBuilder) needUpdate(original []byte) bool {
	if original == nil {
		return true
	}

	return !bytes.Equal(b.payload, original)
}

func (b *psBuilder) buildData(_ []byte) ([]byte, error) {
	return b.payload, nil
}

type propertyPSBuilder struct {
	payload   []byte
	remoteRef esv1beta1.PushRemoteRef
}

func (b *propertyPSBuilder) buildMetadata(annotations, labels map[string]string) (map[string]string, map[string]string, error) {
	newAnnotations := map[string]string{}
	newLabels := map[string]string{}
	if annotations != nil {
		newAnnotations = annotations
	}
	if labels != nil {
		newLabels = labels
	}

	newLabels[managedByKey] = managedByValue
	return newAnnotations, newLabels, nil
}

func (b *propertyPSBuilder) needUpdate(original []byte) bool {
	if original == nil {
		return true
	}

	val := getDataByProperty(original, b.remoteRef.GetProperty())
	return !val.Exists() || val.String() != string(b.payload)
}

func (b *propertyPSBuilder) buildData(original []byte) ([]byte, error) {
	var base []byte
	if original != nil {
		base = original
	}
	return sjson.SetBytes(base, b.remoteRef.GetProperty(), b.payload)
}
