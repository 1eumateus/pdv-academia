// Caminho: pdv/internal/nfe/nfe.go

// Package nfe lê o XML da NF-e (modelo 55) ou da NFC-e (modelo 65, cupom de
// compra no balcão), layout 4.00 — as duas têm a mesma estrutura. Só leitura de arquivo: não valida assinatura, não consulta a
// SEFAZ e não emite nada. Valores viram centavos e quantidades viram inteiros
// com 4 casas, sem float.
package nfe

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxXMLSize = 10 << 20 // uma NF-e real tem poucas centenas de KB

// QtyScale: quantidades vêm com até 4 casas; Qty4 = quantidade × 10000.
const QtyScale = 10000

type Address struct {
	Street, Number, Complement, District, City, State, ZipCode, Phone string
}

type Issuer struct {
	Document          string // CNPJ ou CPF, só dígitos
	LegalName         string
	TradeName         string
	StateRegistration string
	Address           Address
}

type Item struct {
	Number                 int
	Code                   string // cProd: código do produto no fornecedor
	EAN                    string // vazio quando "SEM GTIN"
	Name                   string
	NCM, CFOP              string
	Unit                   string
	Qty4                   int64 // quantidade comercial × 10000
	ProductsCents          int64 // vProd
	FreightCents           int64
	InsuranceAndOtherCents int64 // vSeg + vOutro
	DiscountCents          int64
	IPICents               int64
	STCents                int64
}

type Totals struct {
	ProductsCents, FreightCents, InsuranceAndOtherCents, DiscountCents, IPICents, STCents, InvoiceCents int64
}

type Invoice struct {
	AccessKey string // 44 dígitos
	Number    string
	Series    string
	IssueDate string // AAAA-MM-DD
	Issuer    Issuer
	Items     []Item
	Totals    Totals
}

// ---------- estrutura do XML (só os campos usados) ----------

type xEnder struct {
	XLgr    string `xml:"xLgr"`
	Nro     string `xml:"nro"`
	XCpl    string `xml:"xCpl"`
	XBairro string `xml:"xBairro"`
	XMun    string `xml:"xMun"`
	UF      string `xml:"UF"`
	CEP     string `xml:"CEP"`
	Fone    string `xml:"fone"`
}

// O grupo do ICMS muda de nome conforme a tributação (ICMS00, ICMS10,
// ICMSSN202...); só o valor do ST interessa.
type xICMSGroup struct {
	VICMSST string `xml:"vICMSST"`
}

type xDet struct {
	NItem string `xml:"nItem,attr"`
	Prod  struct {
		CProd  string `xml:"cProd"`
		CEAN   string `xml:"cEAN"`
		XProd  string `xml:"xProd"`
		NCM    string `xml:"NCM"`
		CFOP   string `xml:"CFOP"`
		UCom   string `xml:"uCom"`
		QCom   string `xml:"qCom"`
		VProd  string `xml:"vProd"`
		VFrete string `xml:"vFrete"`
		VSeg   string `xml:"vSeg"`
		VDesc  string `xml:"vDesc"`
		VOutro string `xml:"vOutro"`
	} `xml:"prod"`
	Imposto struct {
		ICMS struct {
			Groups []xICMSGroup `xml:",any"`
		} `xml:"ICMS"`
		IPI struct {
			IPITrib struct {
				VIPI string `xml:"vIPI"`
			} `xml:"IPITrib"`
		} `xml:"IPI"`
	} `xml:"imposto"`
}

type xInfNFe struct {
	ID  string `xml:"Id,attr"`
	Ide struct {
		Mod   string `xml:"mod"`
		Serie string `xml:"serie"`
		NNF   string `xml:"nNF"`
		DhEmi string `xml:"dhEmi"`
		DEmi  string `xml:"dEmi"`
	} `xml:"ide"`
	Emit struct {
		CNPJ      string `xml:"CNPJ"`
		CPF       string `xml:"CPF"`
		XNome     string `xml:"xNome"`
		XFant     string `xml:"xFant"`
		IE        string `xml:"IE"`
		EnderEmit xEnder `xml:"enderEmit"`
	} `xml:"emit"`
	Det   []xDet `xml:"det"`
	Total struct {
		ICMSTot struct {
			VST    string `xml:"vST"`
			VProd  string `xml:"vProd"`
			VFrete string `xml:"vFrete"`
			VSeg   string `xml:"vSeg"`
			VDesc  string `xml:"vDesc"`
			VIPI   string `xml:"vIPI"`
			VOutro string `xml:"vOutro"`
			VNF    string `xml:"vNF"`
		} `xml:"ICMSTot"`
	} `xml:"total"`
}

type xNFe struct {
	InfNFe xInfNFe `xml:"infNFe"`
}

type xNfeProc struct {
	NFe  xNFe `xml:"NFe"`
	Prot struct {
		Inf struct {
			ChNFe string `xml:"chNFe"`
		} `xml:"infProt"`
	} `xml:"protNFe"`
}

// ---------- leitura ----------

var errNotNFe = errors.New("o arquivo não é um XML de NF-e (esperado nfeProc ou NFe)")

// Parse lê uma NF-e (com ou sem o protocolo de autorização).
func Parse(r io.Reader) (*Invoice, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxXMLSize+1))
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler o arquivo: %w", err)
	}
	if len(data) > maxXMLSize {
		return nil, errors.New("o arquivo é grande demais para ser uma NF-e")
	}
	root, err := rootName(data)
	if err != nil {
		return nil, err
	}
	var inf xInfNFe
	var protKey string
	switch root {
	case "nfeProc":
		var p xNfeProc
		if err := xml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("XML inválido: %w", err)
		}
		inf, protKey = p.NFe.InfNFe, p.Prot.Inf.ChNFe
	case "NFe":
		var n xNFe
		if err := xml.Unmarshal(data, &n); err != nil {
			return nil, fmt.Errorf("XML inválido: %w", err)
		}
		inf = n.InfNFe
	default:
		return nil, errNotNFe
	}
	return convert(inf, protKey)
}

func rootName(data []byte) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errNotNFe
			}
			return "", fmt.Errorf("XML inválido: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}

func convert(inf xInfNFe, protKey string) (*Invoice, error) {
	if strings.TrimSpace(inf.Ide.NNF) == "" {
		return nil, errNotNFe
	}
	if m := strings.TrimSpace(inf.Ide.Mod); m != "55" && m != "65" {
		return nil, fmt.Errorf("o documento é do modelo %s; só NF-e (modelo 55) e NFC-e (modelo 65) podem ser importadas", m)
	}
	key := digits(protKey)
	if key == "" {
		key = digits(strings.TrimPrefix(inf.ID, "NFe"))
	}
	if len(key) != 44 {
		return nil, errors.New("chave de acesso da nota não encontrada ou inválida")
	}
	// XMLs "formatados" trazem quebras de linha e espaços dentro dos valores.
	date := strings.TrimSpace(inf.Ide.DhEmi)
	if date == "" {
		date = strings.TrimSpace(inf.Ide.DEmi)
	}
	if len(date) < 10 {
		return nil, errors.New("data de emissão da nota não encontrada")
	}
	e := inf.Emit
	doc := digits(e.CNPJ)
	if doc == "" {
		doc = digits(e.CPF)
	}
	out := &Invoice{AccessKey: key, Number: strings.TrimSpace(inf.Ide.NNF), Series: strings.TrimSpace(inf.Ide.Serie),
		IssueDate: date[:10],
		Issuer: Issuer{Document: doc, LegalName: clean(e.XNome), TradeName: clean(e.XFant), StateRegistration: clean(e.IE),
			Address: Address{Street: clean(e.EnderEmit.XLgr), Number: clean(e.EnderEmit.Nro), Complement: clean(e.EnderEmit.XCpl),
				District: clean(e.EnderEmit.XBairro), City: clean(e.EnderEmit.XMun), State: clean(e.EnderEmit.UF),
				ZipCode: digits(e.EnderEmit.CEP), Phone: digits(e.EnderEmit.Fone)}}}
	if out.Issuer.LegalName == "" {
		return nil, errors.New("emitente da nota não encontrado")
	}
	if len(inf.Det) == 0 {
		return nil, errors.New("a nota não tem itens")
	}

	// money converte um campo de valor; o primeiro erro interrompe a leitura.
	var firstErr error
	money := func(field, v string) int64 {
		c, err := ParseDecimal(v, 2)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("valor inválido em %s (%q)", field, v)
		}
		return c
	}
	for i, d := range inf.Det {
		p := d.Prod
		qty, err := ParseDecimal(p.QCom, 4)
		if err != nil || qty <= 0 {
			return nil, fmt.Errorf("quantidade inválida no item %d (%q)", i+1, p.QCom)
		}
		var st int64
		for _, g := range d.Imposto.ICMS.Groups {
			st += money("vICMSST", g.VICMSST)
		}
		ean := strings.TrimSpace(p.CEAN)
		if strings.EqualFold(ean, "SEM GTIN") {
			ean = ""
		}
		out.Items = append(out.Items, Item{
			Number: i + 1, Code: clean(p.CProd), EAN: ean, Name: clean(p.XProd), NCM: clean(p.NCM), CFOP: clean(p.CFOP),
			Unit: clean(p.UCom), Qty4: qty, ProductsCents: money("vProd", p.VProd), FreightCents: money("vFrete", p.VFrete),
			InsuranceAndOtherCents: money("vSeg", p.VSeg) + money("vOutro", p.VOutro),
			DiscountCents:          money("vDesc", p.VDesc), IPICents: money("vIPI", d.Imposto.IPI.IPITrib.VIPI), STCents: st,
		})
	}
	t := inf.Total.ICMSTot
	out.Totals = Totals{ProductsCents: money("vProd", t.VProd), FreightCents: money("vFrete", t.VFrete),
		InsuranceAndOtherCents: money("vSeg", t.VSeg) + money("vOutro", t.VOutro), DiscountCents: money("vDesc", t.VDesc),
		IPICents: money("vIPI", t.VIPI), STCents: money("vST", t.VST), InvoiceCents: money("vNF", t.VNF)}
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// ParseDecimal converte "123.4567" em inteiro com `places` casas (arredondando
// a casa seguinte para cima a partir de 5). Vazio vale 0. Não aceita negativo.
func ParseDecimal(s string, places int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" || len(intPart) > 13 || digits(intPart) != intPart || digits(frac) != frac {
		return 0, fmt.Errorf("número inválido: %q", s)
	}
	roundUp := len(frac) > places && frac[places] >= '5'
	if len(frac) > places {
		frac = frac[:places]
	}
	frac += strings.Repeat("0", places-len(frac))
	var n int64
	for _, c := range intPart + frac {
		n = n*10 + int64(c-'0')
	}
	if roundUp {
		n++
	}
	return n, nil
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
