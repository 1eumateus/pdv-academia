// Caminho: pdv/internal/service/subscription_import.go

package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"pdv/internal/importer"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// SubscriptionImportService importa assinaturas de planilha (modelos em
// SubscriptionTemplate): a de clientes (código, nome, plano…) e, opcional, a de
// mensalidades (o histórico). Cria os planos que faltam, os clientes, uma
// assinatura por cliente com o histórico de pagamentos (sem virar venda nem
// mexer no caixa) e a mensalidade em aberto. Cliente que já tem assinatura é
// pulado, então dá para importar de novo para incluir clientes novos.
type SubscriptionImportService struct {
	Base
	customers repository.CustomerRepo
	products  repository.ProductRepo
	subs      repository.SubscriptionRepo
}

func NewSubscriptionImportService(b Base) *SubscriptionImportService {
	return &SubscriptionImportService{Base: b}
}

// impClient: uma linha da planilha de clientes (mat = o código do cliente na planilha).
type impClient struct {
	mat, name, cpf, birth, email, street, district, city, uf, zip string
	plan, due                                                     string // modo simples: plano e próximo vencimento
	days, cents                                                   int64  // duração do plano e valor (0 = não informado)
	phones, notes                                                 []string
	active                                                        bool
}

type impInvoice struct {
	mat, plan, due, paidAt string
	cents                  int64
	paid                   bool
}

// Import lê as planilhas (base64); a de mensalidades é opcional (nome vazio).
// apply=false só calcula a prévia.
func (s *SubscriptionImportService) Import(ctx context.Context, clientsName, clientsB64, invoicesName, invoicesB64 string, apply bool) (models.SubscriptionImport, error) {
	var out models.SubscriptionImport
	if _, err := requirePerm(ctx, s.Now(), PermSettings); err != nil {
		return out, err
	}
	cRows, err := readSheet(clientsName, clientsB64)
	if err != nil {
		return out, err
	}
	var iRows [][]string
	if invoicesName != "" {
		if iRows, err = readSheet(invoicesName, invoicesB64); err != nil {
			return out, err
		}
	}
	// Palavras com "�" (acento perdido): deduz pela palavra parecida das planilhas.
	out.FixedWords = fixReplacementChars(append(cRows, iRows...))

	clients, err := parseImportClients(cRows)
	if err != nil {
		return out, err
	}
	invoices := []impInvoice{}
	if iRows != nil {
		if invoices, out.OtherLines, err = parseImportInvoices(iRows); err != nil {
			return out, err
		}
	}
	// Modo simples: cliente sem histórico, com plano, vira uma mensalidade em
	// aberto no próximo vencimento (hoje, se vazio).
	withHistory := map[string]bool{}
	for _, inv := range invoices {
		withHistory[inv.mat] = true
	}
	today := s.Now().In(Location()).Format(dateLayout)
	for _, c := range clients {
		if c.plan != "" && !withHistory[c.mat] {
			invoices = append(invoices, impInvoice{mat: c.mat, plan: c.plan, due: cmpStr(c.due, today), cents: c.cents})
		}
	}

	// Planos: o mesmo nome (sem acento e minúsculo) é o mesmo plano; vale a
	// primeira grafia. Preço = o mais comum; duração = a da planilha de clientes ou 30.
	spelling := map[string]string{}
	planOf := func(p string) string {
		k := foldText(p)
		if n, ok := spelling[k]; ok {
			return n
		}
		spelling[k] = p
		return p
	}
	prices, days := map[string]map[int64]int{}, map[string]int64{}
	for i := range invoices {
		inv := &invoices[i]
		inv.plan = planOf(inv.plan)
		if prices[inv.plan] == nil {
			prices[inv.plan] = map[int64]int{}
		}
		if inv.cents > 0 {
			prices[inv.plan][inv.cents]++
		}
	}
	for _, c := range clients {
		if c.plan != "" && c.days > 0 && days[planOf(c.plan)] == 0 {
			days[planOf(c.plan)] = c.days
		}
	}
	planNames := make([]string, 0, len(prices))
	for n := range prices {
		planNames = append(planNames, n)
	}
	sort.Strings(planNames)

	byMat := map[string][]impInvoice{}
	for _, inv := range invoices {
		byMat[inv.mat] = append(byMat[inv.mat], inv)
	}

	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		// Planos já cadastrados com o mesmo nome são reaproveitados.
		existingPlans := map[string]models.Product{}
		all, err := s.products.List(ctx, tx)
		if err != nil {
			return err
		}
		for _, p := range all {
			if p.PlanDays != nil {
				existingPlans[foldText(p.Name)] = p
			}
		}
		planID, planPrice := map[string]int64{}, map[string]int64{}
		for _, n := range planNames {
			if p, ok := existingPlans[foldText(n)]; ok {
				planID[n], planPrice[n], days[n] = p.ID, p.PriceCents, *p.PlanDays
				out.Plans = append(out.Plans, models.SubscriptionImportPlan{Name: p.Name, PriceCents: p.PriceCents, Days: *p.PlanDays, Invoices: sumCounts(prices[n]), Exists: true})
				continue
			}
			price := modePrice(prices[n])
			if price == 0 {
				return invalid("Informe o valor do plano %q (coluna Valor).", n)
			}
			if days[n] == 0 {
				days[n] = 30
			}
			planPrice[n] = price
			out.Plans = append(out.Plans, models.SubscriptionImportPlan{Name: n, PriceCents: price, Days: days[n], Invoices: sumCounts(prices[n])})
			if !apply {
				continue
			}
			id, err := s.products.Insert(ctx, tx, repository.ProductFields{Name: n, PriceCents: price, Active: true}, now)
			if err != nil {
				return fromRepo(err, nil, "Já existe um produto com o código do plano "+n+".")
			}
			d := days[n]
			if err := s.products.SetNoStock(ctx, tx, id, true, now); err != nil {
				return err
			}
			if err := s.products.SetPlanDays(ctx, tx, id, &d, now); err != nil {
				return err
			}
			planID[n] = id
		}

		// Clientes: o mesmo CPF, ou o mesmo nome + nascimento, já cadastrado é reaproveitado.
		existing := map[string]int64{}
		list, err := s.customers.List(ctx, tx, "")
		if err != nil {
			return err
		}
		for _, c := range list {
			existing[customerKey(c.Name, str(c.BirthDate))] = c.ID
			if c.Document != nil && *c.Document != "" {
				existing["cpf|"+onlyDigits(*c.Document)] = c.ID
			}
		}
		customerID := map[string]int64{}
		for _, c := range clients {
			key := customerKey(c.name, c.birth)
			id, ok := existing[key]
			if !ok && c.cpf != "" {
				id, ok = existing["cpf|"+c.cpf]
			}
			if ok {
				customerID[c.mat] = id
				out.CustomersExisting++
				continue
			}
			in := c.input()
			f, err := validateCustomer(in)
			if err != nil { // tenta sem os campos opcionais que costumam vir errados
				in.Email, in.Document, in.ZipCode = nil, nil, nil
				if f, err = validateCustomer(in); err != nil {
					out.CustomersSkipped++
					if len(out.Skipped) < 30 {
						out.Skipped = append(out.Skipped, fmt.Sprintf("Código %s (%s): %s", c.mat, c.name, err))
					}
					continue
				}
			}
			out.CustomersNew++
			if !apply {
				customerID[c.mat] = -1
				continue
			}
			id, err = s.customers.Insert(ctx, tx, f, now)
			if err != nil {
				return fromRepo(err, nil, "CPF repetido no código "+c.mat+".")
			}
			if c.birth != "" {
				if err := s.customers.SetBirthDate(ctx, tx, id, &c.birth); err != nil {
					return err
				}
			}
			customerID[c.mat] = id
			existing[key] = id
		}

		// Assinaturas: uma por cliente, no plano da última mensalidade.
		active := map[string]bool{}
		for _, c := range clients {
			active[c.mat] = c.active
		}
		mats := make([]string, 0, len(byMat))
		for m := range byMat {
			mats = append(mats, m)
		}
		sort.Strings(mats)
		for _, m := range mats {
			cid, ok := customerID[m]
			if !ok {
				out.InvoicesSkipped += len(byMat[m])
				continue
			}
			// Cliente que já tem assinatura (importação anterior ou cadastro): não duplica.
			if cid > 0 {
				if has, err := s.subs.CustomerHas(ctx, tx, cid); err != nil {
					return err
				} else if has {
					out.CustomersWithSub++
					continue
				}
			}
			invs := byMat[m]
			sort.Slice(invs, func(i, j int) bool { return invs[i].due < invs[j].due })
			last := invs[len(invs)-1]
			plan := last.plan
			var price *int64
			if last.cents != planPrice[plan] && last.cents > 0 {
				p := last.cents
				price = &p
			}
			isActive := active[m]
			// Período coberto: o último vencimento pago (quem paga adiantado cobre até ele).
			// Em aberto mais antiga que esse vencimento não é a mensalidade atual: fica no
			// histórico como "não utilizou" (sem cobrança). A atual é a última em aberto depois dele; se não
			// houver, a próxima vence no último vencimento pago + a duração do plano.
			var unpaid []impInvoice
			lastPaid, lastPaidDue := "", ""
			for _, inv := range invs {
				if inv.paid {
					out.InvoicesPaid++
					lastPaid, lastPaidDue = max(lastPaid, inv.paidAt), max(lastPaidDue, inv.due)
				} else {
					unpaid = append(unpaid, inv)
				}
			}
			current := -1 // índice em unpaid da mensalidade em aberto atual
			if isActive && len(unpaid) > 0 && unpaid[len(unpaid)-1].due > lastPaidDue {
				current = len(unpaid) - 1
			}
			if isActive {
				out.SubsActive++
				out.InvoicesOpen++ // a atual em aberto, ou a próxima gerada
			} else {
				out.SubsCancelled++
			}
			out.InvoicesUnpaidOld += len(unpaid) - min(1, current+1)
			if !apply {
				continue
			}
			cancelled := ""
			if !isActive {
				cancelled = noonUTC(cmpStr(lastPaid, last.due))
			}
			subID, err := s.subs.InsertImported(ctx, tx, cid, planID[plan], price, noonUTC(invs[0].due), cancelled)
			if err != nil {
				return err
			}
			amount := cmpOr(price, planPrice[plan])
			for _, inv := range invs {
				if inv.paid {
					if err := s.subs.InsertImportedInvoice(ctx, tx, subID, inv.due, cmp0(inv.cents, amount), "paga", noonUTC(inv.paidAt), now); err != nil {
						return err
					}
				}
			}
			for i, inv := range unpaid {
				status := "nao_paga"
				if i == current {
					status = "aberta"
				}
				if err := s.subs.InsertImportedInvoice(ctx, tx, subID, inv.due, cmp0(inv.cents, amount), status, "", now); err != nil {
					return err
				}
			}
			if isActive && current < 0 {
				base, _ := time.Parse(dateLayout, cmpStr(lastPaidDue, last.due))
				if err := s.subs.InsertInvoice(ctx, tx, subID, base.AddDate(0, 0, int(days[plan])).Format(dateLayout), amount, now); err != nil {
					return err
				}
			}
		}
		if !apply {
			return nil
		}
		return audit(ctx, tx, now, nil, "assinaturas_importadas", "importacao", nil, nil, map[string]any{
			"clientes_novos": out.CustomersNew, "assinaturas_ativas": out.SubsActive, "assinaturas_canceladas": out.SubsCancelled,
			"faturas_pagas": out.InvoicesPaid, "faturas_nao_pagas": out.InvoicesUnpaidOld})
	})
	out.Applied = apply && err == nil
	return out, err
}

// cmp0: v, ou def se v é zero (mensalidade sem valor na planilha = valor da assinatura).
func cmp0(v, def int64) int64 {
	if v == 0 {
		return def
	}
	return v
}

func readSheet(name, b64 string) ([][]string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(data) == 0 {
		return nil, invalid("Não foi possível ler o arquivo %s.", name)
	}
	rows, err := importer.Read(name, data)
	if err != nil {
		return nil, invalid("Não foi possível ler %s: %s.", name, err)
	}
	if len(rows) < 2 {
		return nil, invalid("A planilha %s está vazia.", name)
	}
	return rows, nil
}

// header devolve a posição de cada coluna pelo nome (sem acento, minúsculo e
// sem a nota entre parênteses, como "(obrigatório)").
func header(row []string, required ...string) (map[string]int, error) {
	idx := map[string]int{}
	for i, h := range row {
		idx[foldText(headerNote.ReplaceAllString(h, ""))] = i
	}
	for _, r := range required {
		if _, ok := idx[r]; !ok {
			return nil, invalid("A planilha não tem a coluna %q. Use a planilha modelo.", r)
		}
	}
	return idx, nil
}

func cell(row []string, idx map[string]int, col string) string {
	if i, ok := idx[col]; ok && i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// yes: "Sim", "S", "Pago"… (sem acento e minúsculo).
func yes(v string) bool {
	switch foldText(v) {
	case "sim", "s", "pago", "paga", "x":
		return true
	}
	return false
}

func parseImportClients(rows [][]string) ([]impClient, error) {
	idx, err := header(rows[0], "codigo", "nome")
	if err != nil {
		return nil, err
	}
	out := []impClient{}
	for _, r := range rows[1:] {
		c := impClient{mat: cell(r, idx, "codigo"), name: titleCase(cell(r, idx, "nome"))}
		if c.mat == "" || c.name == "" {
			continue
		}
		c.cpf = onlyDigits(cell(r, idx, "cpf"))
		c.birth = parseSheetDate(cell(r, idx, "nascimento"))
		for _, col := range []string{"celular", "telefone"} {
			if p := cell(r, idx, col); p != "" {
				c.phones = append(c.phones, p)
			}
		}
		c.email = strings.ToLower(cell(r, idx, "e-mail"))
		c.street, c.district, c.city = cell(r, idx, "endereco"), cell(r, idx, "bairro"), cell(r, idx, "cidade")
		c.uf, c.zip = strings.ToUpper(cell(r, idx, "uf")), onlyDigits(cell(r, idx, "cep"))
		a := foldText(cell(r, idx, "ativo")) // vazio = ativo
		c.active = a != "nao" && a != "n" && a != "inativo"
		c.plan, c.due = cell(r, idx, "plano"), parseSheetDate(cell(r, idx, "proximo vencimento"))
		c.days, _ = strconv.ParseInt(cell(r, idx, "duracao"), 10, 64)
		c.cents, _ = parseMoneyCell(cmpStr(cell(r, idx, "valor"), "0"))
		c.notes = append(c.notes, "Importado da planilha · Código "+c.mat)
		if c.cpf != "" && !ValidCPF(c.cpf) {
			c.notes = append(c.notes, "CPF informado: "+c.cpf)
			c.cpf = ""
		}
		if o := cell(r, idx, "observacao"); o != "" {
			c.notes = append(c.notes, "Obs.: "+o)
		}
		out = append(out, c)
	}
	return out, nil
}

func (c impClient) input() models.CustomerInput {
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	phone := ""
	for _, p := range c.phones { // o primeiro que serve para o WhatsApp
		if whatsappPhone(p) != "" {
			phone = p
			break
		}
	}
	if phone == "" && len(c.phones) > 0 {
		phone = c.phones[0]
	}
	notes := strings.Join(c.notes, " · ")
	if r := []rune(notes); len(r) > maxNotesLen {
		notes = string(r[:maxNotesLen])
	}
	in := models.CustomerInput{Name: c.name, Phone: opt(phone), Document: opt(c.cpf), Email: opt(c.email),
		Street: opt(c.street), District: opt(c.district), City: opt(c.city), State: opt(c.uf), Notes: opt(notes), Active: c.active}
	if len(c.zip) == 8 {
		in.ZipCode = &c.zip
	}
	return in
}

// parseImportInvoices lê o histórico; linha sem vencimento ou com valor inválido
// é ignorada (contada em other).
func parseImportInvoices(rows [][]string) ([]impInvoice, int, error) {
	idx, err := header(rows[0], "codigo", "plano", "vencimento", "valor")
	if err != nil {
		return nil, 0, err
	}
	out, other := []impInvoice{}, 0
	for _, r := range rows[1:] {
		inv := impInvoice{mat: cell(r, idx, "codigo"), plan: cell(r, idx, "plano"), due: parseSheetDate(cell(r, idx, "vencimento"))}
		if inv.mat == "" && inv.plan == "" {
			continue // linha vazia
		}
		cents, err := parseMoneyCell(cell(r, idx, "valor"))
		if err != nil || inv.mat == "" || inv.plan == "" || inv.due == "" {
			other++
			continue
		}
		inv.cents, inv.paid = cents, yes(cell(r, idx, "pago"))
		if inv.paid {
			inv.paidAt = cmpStr(parseSheetDate(cell(r, idx, "pago em")), inv.due)
		}
		out = append(out, inv)
	}
	return out, other, nil
}

// SubscriptionTemplate gera a planilha modelo: kind "clientes" ou "mensalidades".
func SubscriptionTemplate(kind string) ([]byte, error) {
	var head, ex1, ex2 []string
	switch kind {
	case "clientes":
		head = []string{"Código (obrigatório)", "Nome (obrigatório)", "CPF", "Nascimento", "Celular", "Telefone", "E-mail", "Endereço", "Bairro", "Cidade", "UF", "CEP", "Observação",
			"Ativo (Sim/Não)", "Plano", "Duração (dias)", "Valor", "Próximo vencimento"}
		ex1 = []string{"1", "Maria da Silva", "", "15/03/1990", "(94) 98765-4321", "", "maria@exemplo.com", "Rua A, 100", "Centro", "Tucuruí", "PA", "", "", "Sim", "Mensal", "30", "100,00", "10/11/2026"}
		ex2 = []string{"2", "João Souza", "", "", "(94) 99876-5432", "", "", "", "", "", "", "", "Paga valor especial", "Sim", "Trimestral", "90", "250,00", "05/12/2026"}
	case "mensalidades":
		head = []string{"Código (obrigatório)", "Plano (obrigatório)", "Vencimento (obrigatório)", "Valor (obrigatório)", "Pago (Sim/Não)", "Pago em"}
		ex1 = []string{"1", "Mensal", "10/09/2026", "100,00", "Sim", "08/09/2026"}
		ex2 = []string{"1", "Mensal", "10/10/2026", "100,00", "Sim", "10/10/2026"}
	default:
		return nil, invalid("Tipo de modelo inválido.")
	}
	rows := [][]xlsxCell{make([]xlsxCell, len(head)), make([]xlsxCell, len(head)), make([]xlsxCell, len(head))}
	widths := make([]float64, len(head))
	for i := range head {
		rows[0][i], rows[1][i], rows[2][i] = xlsxCell{text: head[i], style: xsHeader}, xlsxCell{text: ex1[i]}, xlsxCell{text: ex2[i]}
		widths[i] = 18
	}
	return writeXLSX(strings.ToUpper(kind[:1])+kind[1:], widths, nil, rows, true)
}

// parseSheetDate aceita AAAA-MM-DD (com ou sem hora), DD/MM/AAAA e o número de data do Excel.
func parseSheetDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 {
		if t, err := time.Parse(dateLayout, s[:10]); err == nil {
			return t.Format(dateLayout)
		}
		if t, err := time.Parse("02/01/2006", s[:10]); err == nil {
			return t.Format(dateLayout)
		}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)).Format(dateLayout)
	}
	return ""
}

// noonUTC grava a data (AAAA-MM-DD) como meio-dia local, no formato do banco.
func noonUTC(d string) string {
	if d == "" {
		return ""
	}
	t, err := time.ParseInLocation(dateLayout, d, Location())
	if err != nil {
		return ""
	}
	return repository.FormatTime(t.Add(12 * time.Hour))
}

func cmpStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func modePrice(m map[int64]int) int64 {
	var best int64
	n := -1
	for p, c := range m {
		if c > n || (c == n && p > best) {
			best, n = p, c
		}
	}
	return best
}

func sumCounts(m map[int64]int) int {
	t := 0
	for _, c := range m {
		t += c
	}
	return t
}

func customerKey(name, birth string) string { return foldText(name) + "|" + birth }

// titleCase: "ADINALDA DOS SANTOS" vira "Adinalda dos Santos".
func titleCase(s string) string {
	small := map[string]bool{"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && small[w] {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// commonWords: palavras com acento comuns em nomes e endereços, para deduzir o "�".
var commonWords = strings.Fields(`JOÃO JOSÉ MARIA CONCEIÇÃO ARAÚJO SÃO GONÇALVES ANTÔNIO ANTÔNIA FÁTIMA LÚCIA MÁRCIA MÁRCIO SÉRGIO FLÁVIA FLÁVIO
	CLÁUDIA CLÁUDIO VITÓRIA GLÓRIA INÊS LETÍCIA PATRÍCIA BÁRBARA DÉBORA JÉSSICA MÔNICA SÔNIA VALÉRIA SIMÕES MAGALHÃES GUIMARÃES
	ASSUNÇÃO NAZARÉ BELÉM PARÁ CARAJÁS TUCURUÍ MARABÁ MUSCULAÇÃO INTERMEDIÁRIO AVALIAÇÃO FÍSICA RENOVAÇÃO PROMOÇÃO CARTÃO
	ÁGUA GÁS LIMÃO AÇÚCAR AVENIDA TRAVESSA NÚMERO`)

// fixReplacementChars troca as palavras com "�" (acento perdido na exportação)
// pela palavra parecida mais comum nas planilhas (ou na lista acima).
// Devolve quantas palavras foram corrigidas.
func fixReplacementChars(rows [][]string) int {
	const bad = '�'
	freq := map[string]int{}
	for _, w := range commonWords {
		freq[w]++
	}
	wordRe := regexp.MustCompile(`[\p{L}\x{FFFD}]+`)
	for _, r := range rows {
		for _, c := range r {
			if !strings.ContainsRune(c, bad) {
				for _, w := range wordRe.FindAllString(strings.ToUpper(c), -1) {
					freq[w]++
				}
			}
		}
	}
	fixed := 0
	cache := map[string]string{}
	guess := func(w string) string {
		if g, ok := cache[w]; ok {
			return g
		}
		target := []rune(strings.ToUpper(w))
		best, bestN := "", 0
		for cand, n := range freq {
			cr := []rune(cand)
			if len(cr) != len(target) || n <= bestN {
				continue
			}
			ok := true
			for i := range cr {
				if target[i] != bad && target[i] != cr[i] {
					ok = false
					break
				}
			}
			if ok {
				best, bestN = cand, n
			}
		}
		cache[w] = best
		return best
	}
	for i, r := range rows {
		for j, c := range r {
			if !strings.ContainsRune(c, bad) {
				continue
			}
			rows[i][j] = wordRe.ReplaceAllStringFunc(c, func(w string) string {
				if !strings.ContainsRune(w, bad) {
					return w
				}
				g := guess(w)
				if g == "" {
					return w
				}
				fixed++
				switch first := []rune(w)[0]; { // mantém a caixa da palavra original
				case w == strings.ToUpper(w):
					return g
				case unicode.IsUpper(first):
					return titleCase(g)
				}
				return strings.ToLower(g)
			})
		}
	}
	return fixed
}
