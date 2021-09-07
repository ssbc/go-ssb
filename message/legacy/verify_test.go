// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	n := len(testMessages)
	if testing.Short() {
		n = min(50, n)
	}
	for i := 1; i < n; i++ {
		hash, _, err := Verify(testMessages[i].Input, nil)
		r.NoError(err, "verify failed")
		a.Equal(testMessages[i].Hash, hash.String(), "hash mismatch %d", i)
	}
}

func TestVerifyBugs(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	type tcase struct {
		msg []byte
		seq int64
		key string
	}
	var tcases = []tcase{
		{
			seq: 1134,
			key: `%bgehbNSgccG25pjpMu9+I5s1LLdL6MAMkgsSGkbvoL8=.sha256`,
			msg: []byte(`{"previous":"%Ou364gh9oMmjRDUaUKeXlVZzYiEdjEz00NEGXaRtnrQ=.sha256","author":"@NaDXehMSIgk08W5RXZJ0p+7m+19iIWEuAtD7FRESJX8=.ed25519","sequence":1134,"timestamp":1515151248938,"hash":"sha256","content":{"type":"post","channel":"alienintelligence","text":"### [THE FIRST POST-KEPLER BRIGHTNESS DIPS OF KIC 8462852](https://arxiv.org/pdf/1801.00732.pdf) \n#### aka alien megastructure (Dyson Swarm/Ring)\n\n> In the case of Tabby's star, the new observations show that it dims more at blue wavelengths than red. Thus, its light is passing through a dust cloud, not being blocked by an alien megastructure in orbit around the star. The new analysis of KIC 8462852 showing these results is to be published in The Astrophysical Journal Letters. It reinforces the conclusions reached by Huan Meng, University of Arizona, Tucson, and collaborators in October 2017. They monitored the star at multiple wavelengths using Nasa's Spitzer and Swift missions, and the Belgian AstroLAB IRIS observatory. These results were published in The Astrophysical Journal.\n\n> The photometric monitoring of KIC 8462852 is the first successful effort via crowd-funding to study an astronomical object.\n\n> Multiband photometry taken during Elsie show its amplitude is chromatic, with depth ratios that are consistent with occultation by optically thin dust with size scales \u001c 1µm, and perhaps with variations ntrinsic to the star.\n\n> KIC 8462852 has captured the imagination of both scientists and the public. To that end, our team strives to make the steps taken to learn more about the star as transparent as possible. Additional constraints on the system will come from the triggered observations taken during the Elsie family of dips and beyond, which will in turn allow for more detailed modeling. Opportunities include observational projects from numerous facilities, impressively demonstrating the multidimensional approach of the community to study KIC 8462852, as mentioned within the above sections. The observed “colors” of the dips (i.e. the ratios of\nthe dip depths in different bands) appear inconsistent with occultation by primarily optically thick material (which would be expected to produce nearly achromatic dips) and appear to be in some tension with intrinsic cooling of the star at constant radius.\n\nOk, so we found out it's uneven ring of dust?\n\n[source](https://science.slashdot.org/story/18/01/04/2352244/the-alien-megastructure-around-mysterious-tabbys-star-is-probably-just-dust-analysis-shows)\n[2](https://en.wikipedia.org/wiki/KIC_8462852)","mentions":[]},"signature":"P9Di8JWeVo9fAIKVkPZiCaib1CjuKYX5EzSqu7lGhpjTeTR/5+Gprsz69fBJGSYWnJdozwfqYh/cRWsfhT55CA==.sig.ed25519"}`),
		},
		{
			seq: 7836,
			key: `%2wLn/3F00bsMSbrbtDmMQR3AFyBTVLszC3bkJ3p+MnY=.sha256`,
			msg: []byte(`{"previous":"%Ym5QnkNCtIHgZG8yk0NBU/ZibTc6qNk1QQov5k5JTl4=.sha256","author":"@f/6sQ6d2CMxRUhLpspgGIulDxDCwYD7DzFzPNr7u5AU=.ed25519","sequence":7836,"timestamp":1508190205432,"hash":"sha256","content":{"type":"npm-packages","mentions":[[null,false]]},"signature":"+uX4y2HwatiR4pvwqIzJL30x4XfTA/MeusQAMI6gT9rawbT5Y7uU40Y8JLgKXKYJtwQ9E5zR70kDYqefbHYVCw==.sig.ed25519"}`),
		},
	}
	for i, tc := range tcases {
		h, dmsg, err := Verify(tc.msg, nil)
		r.NoError(err, "msg %d failed", i)
		a.Equal(tc.key, h.String())
		a.Equal(tc.seq, dmsg.Sequence)
	}
}
