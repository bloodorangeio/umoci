/*
 * umoci: Umoci Modifies Open Containers' Images
 * Copyright (C) 2016-2019 SUSE LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package layer

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openSUSE/umoci/oci/cas/dir"
	"github.com/openSUSE/umoci/oci/casext"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
)

func mustDecodeString(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Ensure that "custom layers" generated by other programs (such as a manual
// tar+gzip) are still correctly handled by us (this used to not work because
// that "archive/tar" parser doesn't consume the whole tar stream if it detects
// that there is no more metadata it is interested in in the tar stream).
func TestUnpackManifestCustomLayer(t *testing.T) {
	ctx := context.Background()

	// These layers were manually generated using GNU tar + GNU gzip.
	// XXX: In future we should also add libarchive tar archives.
	var layers = []struct {
		base64 string
		digest digest.Digest
	}{
		{
			base64: `
H4sIAAsoz1kAA+3XvW7CMBAH8Mx9Cj+Bcz7bZxjYO3brWEXBCCS+lBiJvn2dVColAUpUEop6v8UR
jrGj6P7RyfQl2z/7bOqLUloNJpXJrUFExtRj1BxBaUyUVqQJtSWVgAJnVSL2Nz/JCbsyZEU8yhB7
/UEaxCosVn6iLJAzI3RaIhEhGjJPcTZrzhLUs85Vs/n5tfd+MnYNmfa/R1XjztpqVM5+1r065EGd
Bcf1rxxSImz/RzvUf/6+nceLC/fFhLzwP81wexCylf/Bl+Fttlj6m+3RKf+1ju8freP8H0Qr/62S
qA2hInCt/NcAcgRYvyfbzv+jtfd+MnYN2UO9N32r/5P5r9A16j9exfwfpCb/ef6/zjci36xDsVmW
Isy92GZlOP5ltgu7wkvRvrXwpV+H9nrJxf8oznz/p4sLpdBV9/4Pjebv/yC69X8/fP9HJMdYrR2P
LUfAQ5Bf9d5fI1j3f+A69H/aGuD+jzHGGGOMMcYYY4wxxn7jA5XNY6oAKAAA`,
			digest: digest.NewDigestFromHex(digest.SHA256.String(), "e489a16a8ca0d682394867ad8a8183f0a47cbad80b3134a83412a6796ad9242a"),
		},
		{
			base64: `
H4sIAJ4oz1kAA+3Wu27CMBQG4Mw8xSldK8d3p0OHbu3WN6hCYhELCMh2Bbx9HTogwkWtBLSo51uM
dBLsSPl/heRv5erFlrX1gWimHnOSnRtNtJSbNemvlAmeMcG00FxyajLKqNE0g9XZT3LAR4ilT0e5
xl5/kKAwi25mn5ii2shCKUaKgmshBOWDNC13p5xIvply0U2r4/f+9pOh7yD55ffoMm6U6lZm1Ffu
2bYPNl2wm39muMxAXf5o2/xX60WTfpy4LjXkif/pl9uNIHv9H22I76HybhFJaM6xx8/7XyhjsP+v
4WD/m+JY/2tBKOumRqj9/teUCCXTVAvs/9tALpD3vhRxear/mZC9/Kd3KH3/XSWT/7z/7+/ykWvz
0AwGtmrmMHyNsCwDlDDybtxEqObTGupyDa6F54V30wco2xpiY6GazqtJgKX1FkL0buLacRo4H61t
yRAbACGEEEIIIYQQQgghhBBCCKEr+wTE0sQyACgAAA==`,
			digest: digest.NewDigestFromHex(digest.SHA256.String(), "39f100ed000b187ba74b3132cc207c63ad1765adaeb783aa7f242f1f7b6f5ea2"),
		},
	}

	root, err := ioutil.TempDir("", "umoci-TestUnpackManifestCustomLayer")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	// Create our image.
	image := filepath.Join(root, "image")
	if err := dir.Create(image); err != nil {
		t.Fatal(err)
	}
	engine, err := dir.Open(image)
	if err != nil {
		t.Fatal(err)
	}
	engineExt := casext.NewEngine(engine)

	// Set up the CAS and an image from the above layers.
	var layerDigests []digest.Digest
	var layerDescriptors []ispec.Descriptor
	for _, layer := range layers {
		var layerReader io.Reader

		// Since we already have the digests we don't need to jump through the
		// hoops of decompressing our already-compressed blobs above to get the
		// DiffIDs.
		layerReader = bytes.NewBuffer(mustDecodeString(layer.base64))
		layerDigest, layerSize, err := engineExt.PutBlob(ctx, layerReader)
		if err != nil {
			t.Fatal(err)
		}

		layerDigests = append(layerDigests, layer.digest)
		layerDescriptors = append(layerDescriptors, ispec.Descriptor{
			MediaType: ispec.MediaTypeImageLayerGzip,
			Digest:    layerDigest,
			Size:      layerSize,
		})
	}

	// Create the config.
	config := ispec.Image{
		OS: "linux",
		RootFS: ispec.RootFS{
			Type:    "layers",
			DiffIDs: layerDigests,
		},
	}
	configDigest, configSize, err := engineExt.PutBlobJSON(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	configDescriptor := ispec.Descriptor{
		MediaType: ispec.MediaTypeImageConfig,
		Digest:    configDigest,
		Size:      configSize,
	}

	// Create the manifest.
	manifest := ispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Config: configDescriptor,
		Layers: layerDescriptors,
	}

	bundle, err := ioutil.TempDir("", "umoci-TestUnpackManifestCustomLayer_bundle")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(bundle)

	// Unpack (we map both root and the uid/gid in the archives to the current user).
	mapOptions := &MapOptions{
		UIDMappings: []rspec.LinuxIDMapping{
			{HostID: uint32(os.Geteuid()), ContainerID: 0, Size: 1},
			{HostID: uint32(os.Geteuid()), ContainerID: 1000, Size: 1},
		},
		GIDMappings: []rspec.LinuxIDMapping{
			{HostID: uint32(os.Getegid()), ContainerID: 0, Size: 1},
			{HostID: uint32(os.Getegid()), ContainerID: 100, Size: 1},
		},
		Rootless: os.Geteuid() != 0,
	}
	called := false
	callback := func(m ispec.Manifest, d ispec.Descriptor) error {
		called = true
		return nil
	}
	if err := UnpackManifest(ctx, engineExt, bundle, manifest, mapOptions, callback); err != nil {
		t.Errorf("unexpected UnpackManifest error: %+v\n", err)
	}
	if !called {
		t.Errorf("callback not called")
	}
}
