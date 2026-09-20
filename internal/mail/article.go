package mail

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"elebbs/internal/cfgrec"
	"elebbs/internal/pascal"
)

// NewsArticleSize is SizeOf(NewsArticleRecord) PACKRECORDS 1 (struct.250).
const NewsArticleSize = 521

const (
	ArtTossed = 1 << 0
	ArtEmail  = 1 << 1

	EmailInFile  = "email_in.ele"
	EmailOutFile = "emailout.ele"
	MailLockFile = "runmail.lck"
	MailFwdFile  = "mailfwd.ctl"
	MaxSentTries = 20
	MaxMsgBytes  = 5 * 1024 * 1024
)

// NewsArticle is Pascal NewsArticleRecord (email/news outbound/inbound queue).
type NewsArticle struct {
	GroupName string
	ArticleNr int32
	AreaNum   int32
	BodyLen   int32
	Attribute int32
	TimesSent int32
	Body      []byte
}

func (a NewsArticle) Tossed() bool { return a.Attribute&ArtTossed != 0 }
func (a NewsArticle) Email() bool  { return a.Attribute&ArtEmail != 0 }

func EmailInPath(g *cfgrec.GlobalCfg) string {
	return filepath.Join(g.RaConfig.SysPath, EmailInFile)
}

func EmailOutPath(g *cfgrec.GlobalCfg) string {
	return filepath.Join(g.RaConfig.SysPath, EmailOutFile)
}

func encodeNewsHeader(a NewsArticle) []byte {
	b := make([]byte, NewsArticleSize)
	pascal.PutString(b[0:101], a.GroupName)
	binary.LittleEndian.PutUint32(b[101:], uint32(a.ArticleNr))
	binary.LittleEndian.PutUint32(b[105:], uint32(a.AreaNum))
	binary.LittleEndian.PutUint32(b[109:], uint32(a.BodyLen))
	binary.LittleEndian.PutUint32(b[113:], uint32(a.Attribute))
	binary.LittleEndian.PutUint32(b[117:], uint32(a.TimesSent))
	return b
}

func decodeNewsHeader(b []byte) NewsArticle {
	if len(b) < NewsArticleSize {
		return NewsArticle{}
	}
	return NewsArticle{
		GroupName: pascal.String(b[0:101]),
		ArticleNr: int32(binary.LittleEndian.Uint32(b[101:])),
		AreaNum:   int32(binary.LittleEndian.Uint32(b[105:])),
		BodyLen:   int32(binary.LittleEndian.Uint32(b[109:])),
		Attribute: int32(binary.LittleEndian.Uint32(b[113:])),
		TimesSent: int32(binary.LittleEndian.Uint32(b[117:])),
	}
}

func appendNewsArticle(path string, a NewsArticle) error {
	body := a.Body
	if len(body) == 0 || body[len(body)-1] != 0 {
		body = append(append([]byte{}, body...), 0)
	}
	a.BodyLen = int32(len(body))
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if _, err := f.WriteAt(encodeNewsHeader(a), st.Size()); err != nil {
		return err
	}
	_, err = f.WriteAt(body, st.Size()+NewsArticleSize)
	return err
}

func readNewsFile(path string) ([]NewsArticle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []NewsArticle
	off := 0
	for off+NewsArticleSize <= len(raw) {
		a := decodeNewsHeader(raw[off : off+NewsArticleSize])
		off += NewsArticleSize
		n := int(a.BodyLen)
		if n < 0 {
			return nil, fmt.Errorf("invalid article body length")
		}
		if n > MaxMsgBytes {
			n = MaxMsgBytes
		}
		if off+n > len(raw) {
			n = len(raw) - off
		}
		a.Body = append([]byte{}, raw[off:off+n]...)
		off += int(a.BodyLen)
		if off > len(raw) {
			off = len(raw)
		}
		out = append(out, a)
	}
	return out, nil
}

func rewriteNewsFile(path string, arts []NewsArticle) error {
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if len(arts) == 0 {
		_ = os.Remove(path)
		return nil
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, a := range arts {
		body := a.Body
		if len(body) == 0 || body[len(body)-1] != 0 {
			body = append(append([]byte{}, body...), 0)
		}
		a.BodyLen = int32(len(body))
		if _, err := f.Write(encodeNewsHeader(a)); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
		if _, err := f.Write(body); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

// AddMsgToBase is Pascal AddMsgToBase: append one RFC822 message to a queue file.
func AddMsgToBase(path, groupName string, areaNr int, articleNr int, body []byte, isEmail bool) error {
	attr := int32(0)
	if isEmail {
		attr = ArtEmail
	}
	return appendNewsArticle(path, NewsArticle{
		GroupName: groupName,
		ArticleNr: int32(articleNr),
		AreaNum:   int32(areaNr),
		Attribute: attr,
		Body:      body,
	})
}

// PurgeSentBase drops tossed articles and those that exceeded MaxSent tries.
func PurgeSentBase(path string, maxSent int) (kept int, err error) {
	arts, err := readNewsFile(path)
	if err != nil {
		return 0, err
	}
	var keep []NewsArticle
	for _, a := range arts {
		if a.Tossed() {
			continue
		}
		if maxSent > 0 && int(a.TimesSent) > maxSent {
			continue
		}
		keep = append(keep, a)
	}
	if err := rewriteNewsFile(path, keep); err != nil {
		return 0, err
	}
	return len(keep), nil
}
