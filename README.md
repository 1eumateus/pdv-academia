# PDV Academia — ponto de venda, assinaturas e catraca

Versão do PDV para academia: tudo o que o PDV faz, mais as **assinaturas (planos e mensalidades)** e a **catraca com leitor facial Control iD iDFace**, num programa só e num banco só. O PDV comum (pasta `..\pdv`) não tem assinaturas: elas existem só aqui.

## Academia: assinaturas e catraca

- **Aluno = cliente do PDV.** O plano é um produto com duração (Produtos › Serviços). A assinatura fica em **Academia › Assinaturas**. Receber a mensalidade grava uma venda no caixa e gera a próxima mensalidade. Cancelar ou devolver essa venda estorna o pagamento.
- **Regra da catraca** (`GateDecide` em `internal/service/catraca_service.go`):
  - cliente inativo ou bloqueado na catraca não passa;
  - funcionário passa;
  - com cortesia válida, passa;
  - com plano ativo e não pausado, passa enquanto a mensalidade em aberto não venceu, ou venceu há no máximo a tolerância configurada;
  - a primeira mensalidade de uma assinatura nova precisa ser paga antes de entrar;
  - se o plano tem regras de acesso (`catraca_plano_regras`, em Produtos › Serviços), ou a assinatura tem uma exceção (`catraca_regras`, que vale no lugar das do plano), só passa nos dias da semana marcados, até N dias por semana (seg a dom; quem já entrou hoje não gasta outro) e até N entradas por dia. Funcionário, cortesia e liberação manual ignoram as regras;
  - assinatura pausada, ou cliente sem plano ativo, não passa.
- **Modo online do iDFace:** o programa abre um servidor HTTP (porta 8010 por padrão). A cada rosto reconhecido o aparelho chama `/new_user_identified.fcgi`. O programa decide e responde liberando (com a ação do SecBox) ou negando, com a mensagem na tela do leitor.
- **Quem abre a catraca:** o próprio iDFace (SecBox `id=65793` ou relé interno), uma placa de relé na porta serial (pulso DTR/RTS) ou ninguém (só registra).
- **Telas (menu Academia):**
  - **Catraca:** monitor ao vivo, liberação do aluno no balcão (confere a assinatura) e liberação manual com motivo.
  - **Entradas:** relatório das passagens de um período (busca pelo nome, liberadas/negadas), com totais; Excel leva tudo, PDF o que está na tela. Qualquer usuário logado vê e exporta.
  - **Alunos e rostos:** cadastrar o rosto no leitor (`remote_enroll` com captura automática), ver a foto, remover o rosto, importar quem já está no iDFace, bloquear, dar cortesia e apagar o aluno (tira o rosto do leitor, cancela as assinaturas ativas e desativa o cliente; o histórico fica).
  - **Assinaturas:** também libera a catraca do aluno (confere a assinatura) e reativa uma assinatura cancelada (ativa o cliente; a mensalidade vence hoje).
  - **Funcionários** (permissão `catraca`): quem passa sempre, sem plano. Não são os usuários do sistema; por baixo, cliente com `catraca_alunos.funcao` preenchida. Desligar volta a pessoa a cliente comum.
  - **Configurar catraca.**
- **Aviso com foto:** toda passagem fica registrada (`catraca_passagens`) e aparece num aviso no alto da tela, com a foto guardada no iDFace.
- **Permissão `catraca`** (Usuários): configurar, bloquear, dar cortesia, remover rosto e importar. Cadastrar rosto e liberar o aluno em dia não precisam dela.
- **Tabelas da catraca** (`internal/db/catraca.go`) têm numeração própria (`catraca_versao`), separada do `PRAGMA user_version` do PDV. Assim, uma migration nova do PDV pode ser trazida para cá sem renumerar nada.
- **Apagar vendas de teste** (Administração, só o administrador): apaga de vez as vendas de um dia (até hoje) de um operador, ou de todos, e o caixa que ele abriu nesse dia (`PurgeService`); caixa com outras coisas fica e perde só os lançamentos dessas vendas. Os gatilhos `*_no_delete` saem e voltam na mesma transação; antes, um backup vai para `backups/`. Devolução ou fiado já recebido dessas vendas bloqueiam.
- **Mesma pasta de dados do PDV** (`%APPDATA%\pdv`). Uma academia que já usava o PDV com assinaturas continua com os mesmos dados.

### Instalação no PC da academia

1. Instale o `academia.exe`. Libere a porta 8010 no Firewall do Windows (rede privada).
2. Em **Academia › Configurar catraca**, informe IP, usuário e senha do iDFace e use **Testar iDFace**.
3. No iDFace, ative o **modo online** apontando para o IP deste PC, porta 8010.
4. Em **Academia › Alunos e rostos**, use **Importar do iDFace** para ligar aos clientes quem já está cadastrado no aparelho. A ligação é feita pela matrícula `C<código do cliente>` ou pelo nome igual. Quem não for encontrado vira cliente novo. Depois, cadastre o rosto dos demais.

### Desenvolvimento

```powershell
wails dev
go test ./...
$env:CGO_ENABLED = "0"; wails build -clean     # gera buildincademia.exe
```

---


Sistema de ponto de venda para Windows, feito para pequenos comércios que vendem produtos por unidade (mercearias, lojas de roupas, papelarias, lojas de acessórios etc.). Funciona sem internet, em uma máquina por estabelecimento, com banco SQLite local.

> **Aviso importante:** este sistema **não emite documentos fiscais** (NFC-e, NF-e, SAT ou qualquer outro) e **não se integra** a bancos, Pix ou maquininhas de cartão. As formas "Pix" e "Cartão" são apenas registradas pelo operador. O comprovante impresso é **não fiscal** e não substitui documento fiscal. Essas integrações são funcionalidades futuras e exigem implementação e homologação específicas.

## Funcionalidades

- **Vendas**: busca por código de barras (leitor) ou nome, carrinho com alteração de quantidades, desconto, formas de pagamento (dinheiro, Pix, cartão, fiado), bloqueio de clique duplo, atalhos `F1` (ir para vendas), `F2` (buscar) e `F4` (finalizar), comprovante não fiscal imprimível.
- **Produtos e estoque**: cadastro, edição, ativação/desativação, código de barras único, entrada, ajuste (+/−), devolução, histórico de movimentações, auditoria de divergências e procedimento de correção.
- **Clientes e fiado**: cadastro (só o nome é obrigatório; CPF/CNPJ validado, inclusive o CNPJ alfanumérico, e-mail, endereço completo, ponto de referência e observações), limite de crédito, saldo devedor, histórico de compras a prazo e de recebimentos, recebimento parcial ou total, estorno auditável de recebimento.
- **Comandas**: abertura por mesa/balcão/código, lançamento e alteração de itens, fechamento atômico que gera uma venda normal, descarte de comanda vazia.
- **Caixa**: abertura com saldo inicial, suprimento e sangria, resumo por forma de pagamento, fechamento com valor contado e diferença, histórico de sessões.
- **Histórico e relatórios**: vendas por período, detalhes, produtos vendidos, totais por forma de pagamento, cancelamentos, movimentações de estoque, recebimentos de fiado, resumo diário.
- **Cancelamento de venda** auditável (motivo, data, reversão de estoque e de crédito, estorno no caixa).
- **Backup e restauração** seguros para SQLite em modo WAL.

## Arquitetura

```
Frontend Vue 3 → bindings Wails → app.go → service → repository → SQLite
```

| Camada | Responsabilidade |
|---|---|
| `frontend/` | Telas, coleta de dados, chamadas aos bindings gerados pelo Wails. Nenhuma regra de negócio crítica. |
| `app.go` | Expõe os métodos ao frontend, encaminha aos serviços e converte erros para o contrato `{code, message}`. |
| `internal/service` | Regras de negócio, validações e controle de transações. |
| `internal/repository` | SQL parametrizado. Recebe um executor (`DBTX`) que pode ser `*sql.DB` ou `*sql.Tx`. |
| `internal/db` | Abertura do banco, PRAGMAs e migrations. |
| `internal/models` | Estruturas de domínio e DTOs. |
| `internal/applog` | Log técnico local. |

Decisões principais:

- **Dinheiro em centavos** (`int64` no Go, `INTEGER` no SQLite). O frontend só formata (`Intl.NumberFormat('pt-BR', {currency: 'BRL'})`) e converte texto para centavos usando apenas inteiros. O total confiável de qualquer venda é calculado no backend com os preços do banco.
- **Uma transação por operação**: venda, fechamento de comanda, cancelamento, recebimento e movimentação de estoque gravam tudo ou nada. Os repositórios usam o mesmo `*sql.Tx` que o serviço abriu.
- **Regra única de escrita do estoque**: toda alteração passa por `applyStockChange`, que atualiza `products.stock_qty` com `UPDATE ... WHERE stock_qty + ? >= 0` (atualização condicional, verificando as linhas afetadas) e insere em `stock_movements` na mesma transação. O saldo inicial também é uma movimentação.
- **Concorrência**: as transações começam com `BEGIN IMMEDIATE` e o pool usa uma conexão (ver abaixo). Duas tentativas simultâneas de vender o último item resultam em uma venda e um erro de estoque insuficiente (há teste para isso).
- **Cliques repetidos**: cada carrinho envia uma chave de idempotência (`requestId`). Reenviar o mesmo pedido devolve a venda já registrada em vez de criar outra. O mesmo vale para recebimentos de fiado. Não há retentativa automática no backend.
- **Histórico imutável**: triggers no banco impedem alterar ou apagar movimentações de estoque, itens de venda, vendas, recebimentos, movimentações de caixa e comandas fechadas. Correções são feitas por novos lançamentos (cancelamento, estorno, ajuste).
- **Erros tipados**: `internal/service/errors.go` define códigos estáveis (`ESTOQUE_INSUFICIENTE`, `LIMITE_CREDITO_EXCEDIDO`, `CAIXA_NAO_ABERTO`...), compatíveis com `errors.Is`/`errors.As`. Na fronteira Go→JS, todo erro vira um JSON `{"code": "...", "message": "..."}` (`app.go`, `ErrorPayload`) e o frontend decide o fluxo pelo `code` (`frontend/src/api.ts`). Erros técnicos aparecem com mensagem genérica e o detalhe vai para o log.

### Regras de negócio definidas

- **Caixa**: vendas (qualquer forma), fechamento de comanda, recebimentos e estornos exigem caixa aberto. Vendas em dinheiro, Pix e cartão geram movimentação no caixa; vendas a prazo não. O "dinheiro esperado" considera só dinheiro: saldo inicial + vendas + recebimentos + suprimentos − sangrias − estornos.
- **Fiado**: exige cliente ativo; `dívida atual + valor da venda <= limite`, checado de forma condicional dentro da transação. O recebimento não pode passar da dívida, não gera movimentação de estoque e não altera a venda original.
- **Cancelamento**: devolve o estoque (movimentação `cancelamento`). No fiado, abate a dívida; se o cliente já pagou parte e a dívida atual é menor que o total da venda, o cancelamento é recusado: estorne os recebimentos antes. Em dinheiro/Pix/cartão, registra um estorno no caixa (obrigação de reembolso feito pelo operador; o sistema não faz devolução bancária). Uma venda cancelada não pode ser cancelada de novo.
- **Comandas**: o preço do item é gravado no lançamento e é o preço cobrado no fechamento. O estoque é baixado no fechamento. Um produto desativado depois do lançamento continua visível na comanda, sinalizado, e impede o fechamento até ser removido. Comanda vazia não pode ser fechada; pode ser descartada. Ao cancelar uma venda originada de comanda, a comanda continua fechada (histórico preservado) e a venda guarda o vínculo `comanda_id`.
- **Produtos e clientes** não são excluídos fisicamente: são desativados.
- **Divergência de estoque** (só surge se o banco for alterado por fora): a auditoria compara o saldo com a soma das movimentações. A correção registra um `ajuste` com referência `reconciliacao` para que o histórico volte a explicar o saldo, sem apagar nada. Depois, recomenda-se contar o produto e fazer um ajuste se a contagem física for diferente.

## Estrutura de diretórios

```
pdv/
├── go.mod / go.sum
├── main.go                  inicialização, restauração pendente, Wails
├── app.go                   métodos expostos ao frontend
├── wails.json
├── build/                   ícones e manifesto do Windows (gerados pelo Wails)
├── internal/
│   ├── applog/applog.go
│   ├── db/                  database.go, migrations.go, db_test.go
│   ├── models/models.go
│   ├── repository/          repository.go (DBTX, erros), *_repo.go
│   └── service/             errors.go, service.go, *_service.go e testes
└── frontend/
    ├── package.json / package-lock.json / vite.config.ts / tsconfig.json / index.html
    ├── wailsjs/             bindings gerados pelo Wails (não editar)
    └── src/
        ├── main.ts, App.vue, api.ts
        ├── router/index.ts
        ├── views/           Vendas, Produtos, Clientes, Comandas, Caixa, HistoricoVendas, Relatorios, Configuracoes
        ├── components/      ProductSearch, CustomerPicker, Receipt, SaleDetailsModal, Modal, ConfirmDialog, ToastHost
        ├── composables/     format.ts, toast.ts, cashState.ts
        ├── types/index.ts
        └── assets/main.css
```

Desvios da estrutura de referência, com justificativa:
- `internal/repository/repository.go` e `settings_repo.go`, `internal/service/service.go` e `settings_service.go`: base comum (executor, classificação de erros, transação, validações) e a tela de Configurações.
- **Aparência** (Configurações, permissão `alterar_configuracoes`): imagem de fundo da tela de login e do sistema. A tela reduz a foto para até 1920 px em JPEG; fica na tabela `settings` (`bg_login`, `bg_app`), entra no backup e é lida fora de `GetSettings` (`GetBackgrounds`, sem login).
- `internal/applog`: log local controlado.
- Os testes ficam junto aos pacotes (`*_test.go`).

## Versões e dependências

| Componente | Versão |
|---|---|
| Go | 1.26+ (`go.mod`); verificado com 1.27.2 |
| Wails | v2.16.0 |
| SQLite | `modernc.org/sqlite` v1.60.1 (Go puro, **sem CGO**) |
| Node.js / npm | verificado com 24.20 / 11.19 |
| Vue / Vue Router | 3.5.44 / 4.6.4 |
| Vite / TypeScript / vue-tsc | 7.3.7 / 5.9.3 / 2.2.12 |

Não há ORM nem bibliotecas de UI. As versões do frontend estão fixadas em `package.json` e `package-lock.json`.

## Pré-requisitos para desenvolvimento (Windows)

1. **Go** 1.26 ou superior: <https://go.dev/dl/>
2. **Node.js** LTS com npm: <https://nodejs.org/>
3. **WebView2 Runtime** (já vem no Windows 10/11 atualizados)
4. **Wails CLI**:
   ```powershell
   go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
   wails doctor
   ```
   Garanta que `%USERPROFILE%\go\bin` está no `PATH`.

## Instalação das dependências

```powershell
cd pdv
go mod download
cd frontend
npm ci
cd ..
```

## Execução em desenvolvimento

```powershell
wails dev
```

Para não misturar dados de teste com o banco real, aponte para outra pasta:

```powershell
$env:PDV_DATA_DIR = "C:\temp\pdv-teste"; wails dev
```

## Compilação

```powershell
$env:CGO_ENABLED = "0"
wails build -clean
```

O executável é gerado em `build\bin\pdv.exe`. Para gerar um instalador NSIS (requer o NSIS instalado): `wails build -clean -nsis`.

## Instalação na máquina do estabelecimento

1. Copie `pdv.exe` para uma pasta, por exemplo `C:\Program Files\PDV\` (ou use o instalador NSIS).
2. Confirme que o **Microsoft Edge WebView2 Runtime** está instalado (padrão no Windows 10/11; se faltar, baixe em <https://developer.microsoft.com/microsoft-edge/webview2/>).
3. Crie um atalho na área de trabalho e abra o programa. O banco é criado automaticamente na primeira execução.
4. Em **Configurações**, informe o nome do estabelecimento. Em **Caixa**, abra o caixa para começar a vender.

Requisitos: Windows 10 ou 11, 64 bits. Não requer internet nem permissões de administrador para rodar.

## Localização do banco de dados

```
%APPDATA%\pdv\pdv.db      (ex.: C:\Users\<usuário>\AppData\Roaming\pdv\pdv.db)
```

O caminho vem de `os.UserConfigDir()`. O banco nunca fica na pasta do executável, que pode não ter permissão de gravação e pode ser substituída numa atualização. Na mesma pasta ficam `pdv.db-wal` e `pdv.db-shm` (arquivos do modo WAL, **não apague**), `logs\pdv.log` e `backups\`. O caminho exato aparece em **Configurações → Sobre e diagnóstico**.

### Configuração do SQLite

- Os PRAGMAs vão na DSN (`_pragma=...`), então o driver os aplica em **toda** conexão do pool: `foreign_keys=ON`, `busy_timeout=5000`, `journal_mode=WAL`, `synchronous=NORMAL`. Ao abrir, o programa confere se as chaves estrangeiras e o WAL estão ativos.
- `_txlock=immediate`: as transações começam com `BEGIN IMMEDIATE`, reservando a escrita logo no início e evitando o deadlock de promoção de leitura para escrita.
- **Pool com uma conexão** (`SetMaxOpenConns(1)`): num PDV de uma máquina o volume é baixo, as escritas ficam serializadas no próprio Go e a aplicação não convive com `SQLITE_BUSY`. O `busy_timeout` protege contra processos externos (por exemplo, um visualizador de banco aberto por um técnico).
- WAL: leitores não bloqueiam o escritor e o banco tolera melhor quedas de energia. `synchronous=NORMAL` em WAL mantém a consistência; numa queda de energia, as transações confirmadas nos últimos instantes podem se perder, mas o banco não corrompe.
- Erros de bloqueio, falta de espaço e leitura/escrita são classificados (`repository.ErrBusy`/`ErrStorage`) e mostrados ao usuário com mensagens claras.

## Migrations

O versionamento usa `PRAGMA user_version`. Na abertura, o programa:

1. lê a versão atual;
2. recusa bancos de versão **mais nova** que a conhecida (não modifica um banco incompatível);
3. aplica, em ordem, apenas as migrations pendentes, cada uma em transação (DDL e `user_version` são transacionais no SQLite);
4. só avança `user_version` depois que a migration inteira deu certo; se algo falha, faz rollback e devolve um erro contextualizado, sem apagar dados.

| Versão | Conteúdo |
|---|---|
| 1 | produtos, clientes, comandas e itens, vendas e itens, movimentações de estoque, índices, CHECKs e triggers de imutabilidade |
| 2 | recebimentos de crédito (`credit_payments`, com estorno auditável) |
| 3 | sessões e movimentações de caixa, `sales.cash_session_id`, configurações |
| 4 | dados opcionais do cliente: CPF/CNPJ (único quando informado), e-mail e endereço de entrega |

Para evoluir o esquema, **acrescente** uma migration ao final de `internal/db/migrations.go`; nunca altere uma migration já distribuída.

## Backup e restauração

- **Criar backup** (Configurações → Criar backup): o programa usa `VACUUM INTO`, que gera, a partir da conexão ativa, uma cópia íntegra que **já inclui o conteúdo do arquivo WAL**. A cópia é gravada num arquivo temporário, verificada (`integrity_check`, `foreign_key_check`, versão do esquema, tabelas obrigatórias) e só então renomeada para o destino escolhido.
- **Restaurar** (Configurações → Restaurar backup): o arquivo escolhido é validado e preparado como `pdv.db.restore`. O banco em uso **não é tocado** com o programa aberto. Feche o programa (há um botão para isso); na próxima abertura, antes de abrir o banco, o programa:
  1. valida o arquivo novamente;
  2. move o banco atual (com `-wal` e `-shm`) para `backups\antes-da-restauracao-<data>\`;
  3. coloca o backup no lugar, abre, aplica migrations e verifica a integridade;
  4. se algo falhar, devolve os arquivos originais e informa na tela.
- Arquivos inválidos, corrompidos, de outro programa ou de versão mais nova são recusados.

**Limites da recuperação:** o backup só protege o que foi copiado. Guarde cópias **fora deste computador** (pen drive, outro disco, nuvem). Se o disco falhar e não houver cópia externa, os dados se perdem. Vendas feitas depois do último backup não estão nele.

## Testes

```powershell
$env:CGO_ENABLED = "0"
go test ./...
go vet ./...
```

Os testes usam SQLite temporário e não dependem de internet. Eles verificam o **estado final do banco** (contagens, saldos, estoque), e não apenas o retorno das funções. Falhas no meio de transações são simuladas com triggers que abortam a operação. Cobertura:

- **Banco e migrations**: banco vazio, PRAGMAs em todas as conexões do pool, atualização da versão 1 para a atual preservando dados, rollback de migration com falha, recusa de versão futura, chaves estrangeiras, imutabilidade do histórico.
- **Produtos e estoque**: cadastro, edição, busca, código de barras duplicado, entrada, ajuste positivo e negativo, estoque negativo, auditoria e correção de divergência.
- **Vendas**: sucesso, múltiplos itens consolidados, cálculo com desconto, snapshot de nome e preço, validações, produto inativo, caixa fechado, mensagem de estoque insuficiente, rollback em falha de item / movimentação / atualização de estoque / efeito financeiro, 10 vendas concorrentes do último item, cliques repetidos (idempotência), proteção contra estouro.
- **Crédito**: venda a prazo válida, sem cliente, limite excedido e exato, cliente inativo, recebimento parcial e total, pagamento acima da dívida, falha entre o registro e a atualização do saldo, idempotência, estorno, cancelamento de venda a prazo.
- **Comandas**: abertura, mesa duplicada, adição e soma de itens, preço preservado, fechamento, fechamento duplicado, item após fechamento, estoque insuficiente no fechamento, rollback, comanda vazia, produto desativado, descarte, limite de crédito.
- **Cancelamentos e caixa**: cancelamento, duplo cancelamento, reversão de estoque e crédito, estorno no caixa, rollback do cancelamento, abertura, caixa duplicado, suprimento, sangria (inclusive acima do saldo), resumo por forma, fechamento com diferença, venda após fechamento.
- **Relatórios** e **backup** (criação, validação, restauração com conferência dos dados, rejeição de arquivos inválidos e de restauração corrompida).

## Segurança e dados armazenados

- Ficam no computador: cadastro de produtos e clientes (nome, telefone, CPF/CNPJ, e-mail, endereço, limite, dívida), vendas, estoque, caixa e configurações.
- **Os dados não são criptografados** e **não há login nem controle de usuários** nesta versão. Qualquer pessoa com acesso à conta do Windows pode abrir o programa e ler o arquivo do banco. Num computador compartilhado, use uma conta do Windows exclusiva para o PDV, com senha.
- O backend valida todas as entradas, usa SQL parametrizado e restrições no banco, e não expõe detalhes do banco nas mensagens. Caminhos de backup e restauração são escolhidos em diálogos nativos e validados (caminho absoluto, extensão `.db`, diferente do banco ativo).
- O log técnico (`logs\pdv.log`, rotacionado aos 5 MB) registra erros técnicos para suporte.

## Limitações conhecidas

- Uma máquina por estabelecimento; sem sincronização entre computadores.
- Sem emissão fiscal e sem integração com meios de pagamento.
- Sem usuários, permissões ou autenticação.
- Quantidades somente inteiras (sem venda por peso).
- A impressão do comprovante usa a impressão do sistema (`window.print()`); não há driver específico de impressora térmica.
- A restauração de backup exige reiniciar o programa.
- Datas dos relatórios usam o fuso horário configurado no Windows.

## Funcionalidades futuras

Emissão fiscal (NFC-e/SAT), integração TEF/Pix, impressora térmica (ESC/POS), usuários e permissões aplicados no backend, venda por peso, backup automático agendado, sincronização multi-loja.
