package catalog

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCanonicalSourceBytesRemainReviewed(t *testing.T) {
	for _, source := range []struct{ text, digest string }{
		{MetadataSQL, "382078e4fd0754e9d8ec268bae9f00a0f89c805dafa711dea98a3b5f46246b2e"},
		{CellsTemplate, "ed5314cfa18878032eb4e8feb0c11d2f40be0236d9129a92f9ee7c08a7cdc409"},
		{XMLTemplate, "39bd0662d65a30cc30a0ea182beba7fcbe86e003b0b24714b5ed9bcb36b0e16d"},
		{ScalarAlertsTemplate, "a42dfd9f52aa753e35aff0b00239e8705e173616a818b9a39f69fdcbf47bb3ef"},
	} {
		if fmt.Sprintf("%x", sha256.Sum256([]byte(source.text))) != source.digest {
			t.Fatal("shared canonical SQL changed without review")
		}
	}
}
