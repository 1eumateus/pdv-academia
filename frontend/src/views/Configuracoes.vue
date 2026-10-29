<!-- Caminho: pdv/frontend/src/views/Configuracoes.vue -->
<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  CancelPendingRestore, ChooseBackupFolder, CreateBackup, GetAppInfo, GetBackupStatus, GetSettings, HasPendingRestore, ListPrinters,
  OpenLogsFolder, QuitApp, RestoreBackup, RunBackupNow, SaveSettings,
} from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { can } from '../composables/session'
import BackgroundSettings from '../components/BackgroundSettings.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import { refreshSettings } from '../composables/cashState'
import { formatDateTime } from '../composables/format'
import { directPrint, printReceipt } from '../composables/receipt'
import { useToast } from '../composables/toast'
import { models, type AppInfo, type Settings } from '../types'

const toast = useToast()
// form espelha models.Settings; o backend valida e normaliza tudo ao salvar.
const form = reactive<Settings>(models.Settings.createFrom({
  paperWidth: '80', cutPaper: true, quoteValidityDays: 7, operatorMaxDiscountPct: 10, lowStockDefault: 5,
  nfeCostFreight: true, nfeCostIpi: true, nfeCostSt: true, nfeCostOther: true, nfeCostDiscount: true,
  backupAuto: true, backupKeep: 7, backupWarnDays: 3,
}))
const settings = reactive({ busy: false, error: '' })
const printers = ref<string[]>([])
const testOpen = ref(false)
const MAX_LOGO_BYTES = 300 * 1024

async function loadPrinters() {
  try {
    printers.value = await call(ListPrinters())
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

function pickLogo(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!['image/png', 'image/jpeg'].includes(file.type)) return toast.error('A logo deve ser PNG ou JPEG.')
  if (file.size > MAX_LOGO_BYTES) return toast.error('A logo é grande demais. Use uma imagem de até 300 KB.')
  const reader = new FileReader()
  reader.onload = () => (form.logo = String(reader.result))
  reader.readAsDataURL(file)
}
const info = ref<AppInfo | null>(null)
async function openLogs() {
  try {
    await call(OpenLogsFolder())
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
const pending = ref(false)
const backupBusy = ref(false)
// ---------- backup automático e cópia externa ----------
const autoStatus = ref<models.BackupStatus | null>(null)
async function loadAutoStatus() {
  try {
    autoStatus.value = await call(GetBackupStatus())
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
async function chooseFolder(field: 'backupDir' | 'backupExternalDir') {
  try {
    const dir = await call(ChooseBackupFolder())
    if (dir) form[field] = dir
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
// Salva primeiro (a pasta escolhida pode ainda não estar gravada) e então copia.
async function backupNow() {
  if (can('alterar_configuracoes') && !(await save())) return toast.error(settings.error)
  backupBusy.value = true
  try {
    autoStatus.value = await call(RunBackupNow())
    if (autoStatus.value.externalError) toast.error(`Backup feito, mas a cópia externa falhou: ${autoStatus.value.externalError}`)
    else toast.success(autoStatus.value.externalDir ? 'Backup feito e copiado para a pasta externa.' : 'Backup feito.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    backupBusy.value = false
  }
}
const lastBackup = ref('')
const confirmRestore = ref(false)
const restoreScheduled = ref(false)

onMounted(async () => {
  try {
    info.value = await GetAppInfo()
    pending.value = await HasPendingRestore()
    Object.assign(form, await call(GetSettings()))
    loadPrinters()
    loadAutoStatus()
  } catch (e) {
    settings.error = errorMessage(e)
  }
})

async function save(): Promise<boolean> {
  settings.busy = true
  settings.error = ''
  try {
    const input = models.Settings.createFrom({ ...form })
    Object.assign(form, await call( SaveSettings(input)))
    await refreshSettings()
    loadAutoStatus()
    toast.success('Configurações salvas.')
    return true
  } catch (e) {
    settings.error = errorMessage(e)
    return false
  } finally {
    settings.busy = false
  }
}

// O teste usa a configuração salva, então salva antes.
async function testPrint() {
  if (!(await save())) return
  if (directPrint.value) printReceipt('teste', 0)
  else testOpen.value = true
}

async function backup() {
  backupBusy.value = true
  try {
    const r = await call(CreateBackup())
    if (r) {
      lastBackup.value = `Backup salvo em ${r.path} (${(r.sizeBytes / 1024).toFixed(0)} KB) e verificado.`
      toast.success('Backup criado e verificado.')
    }
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    backupBusy.value = false
  }
}

async function restore() {
  confirmRestore.value = false
  backupBusy.value = true
  try {
    const ok = await call( RestoreBackup())
    if (ok) {
      pending.value = true
      restoreScheduled.value = true
    }
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    backupBusy.value = false
  }
}

async function cancelRestore() {
  try {
    await call(CancelPendingRestore())
    pending.value = false
    restoreScheduled.value = false
    toast.info('Restauração cancelada.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div class="grid grid-2">
    <section v-if="can('alterar_configuracoes')" class="card stack">
      <div class="row between"><h2 style="margin: 0">Dados da loja</h2>
        <button type="button" class="link" @click="$router.push('/assistente')">Abrir o assistente de configuração</button></div>
      <p class="small muted" style="margin: 0">Aparecem no cabeçalho dos comprovantes. Só o nome fantasia é obrigatório.</p>
      <form id="settings-form" class="stack" @submit.prevent="save">
        <div class="grid grid-2">
          <label class="field">Nome fantasia<input v-model="form.storeName" maxlength="200" required /></label>
          <label class="field">Razão social<input v-model="form.legalName" maxlength="200" /></label>
          <label class="field">CNPJ ou CPF<input v-model="form.document" maxlength="20" placeholder="00.000.000/0000-00" /></label>
          <label class="field">Inscrição estadual<input v-model="form.stateRegistration" maxlength="20" /></label>
          <label class="field">Telefone<input v-model="form.phone" maxlength="30" inputmode="tel" /></label>
          <label class="field">WhatsApp<input v-model="form.whatsapp" maxlength="30" inputmode="tel" /></label>
          <label class="field">E-mail<input v-model="form.email" maxlength="254" type="email" /></label>
          <label class="field">CEP<input v-model="form.zipCode" maxlength="10" inputmode="numeric" placeholder="00000-000" /></label>
          <label class="field">Rua<input v-model="form.street" maxlength="200" /></label>
          <label class="field">Número<input v-model="form.number" maxlength="20" /></label>
          <label class="field">Complemento<input v-model="form.complement" maxlength="200" /></label>
          <label class="field">Bairro<input v-model="form.district" maxlength="200" /></label>
          <label class="field">Cidade<input v-model="form.city" maxlength="200" /></label>
          <label class="field">UF<input v-model="form.state" maxlength="2" placeholder="SP" /></label>
        </div>
        <div class="row">
          <img v-if="form.logo" :src="form.logo" alt="Logo da loja" style="max-height: 64px; max-width: 160px" />
          <label class="btn">
            {{ form.logo ? 'Trocar logo' : 'Escolher logo' }}
            <input type="file" accept="image/png,image/jpeg" hidden @change="pickLogo" />
          </label>
          <button v-if="form.logo" type="button" class="link" @click="form.logo = ''">Remover logo</button>
          <span class="small muted">PNG ou JPEG, até 300 KB. Na térmica sai em preto e branco.</span>
        </div>
        <label class="field">Mensagem do rodapé do comprovante<input v-model="form.receiptFooter" maxlength="300" placeholder="Obrigado pela preferência!" /></label>
        <label class="field">Mensagem de cobrança do a prazo (WhatsApp)
          <textarea v-model="form.chargeMessage" rows="3" maxlength="1000" />
          <span class="small muted">Use {nome} (primeiro nome do cliente), {saldo} (valor em aberto) e {loja} (nome da loja).</span>
        </label>
        <details class="msgs">
          <summary><strong>Mensagens do WhatsApp: aniversário e assinaturas</strong></summary>
          <p class="small muted">Use {nome} (primeiro nome), {plano}, {vencimento}, {valor} e {loja}. O operador confere e envia.</p>
          <label class="field">Parabéns (aniversariantes)<textarea v-model="form.msgBirthday" rows="2" maxlength="1000" /></label>
          <div class="grid grid-2">
            <label class="field">Mensalidade a vencer<textarea v-model="form.msgDue" rows="3" maxlength="1000" /></label>
            <label class="field">Mensalidade vencida<textarea v-model="form.msgOverdue" rows="3" maxlength="1000" /></label>
            <label class="field">Aluno inativo<textarea v-model="form.msgMissing" rows="3" maxlength="1000" /></label>
            <label class="field">Assinatura cancelada (chamar de volta)<textarea v-model="form.msgWinback" rows="3" maxlength="1000" /></label>
            <label class="field">"A vencer" = vence nos próximos (dias)<input v-model.number="form.dueDays" type="number" min="1" max="60" /></label>
            <label class="field">"Inativo" = vencida há mais de (dias)<input v-model.number="form.missingDays" type="number" min="1" max="365" /></label>
          </div>
        </details>
        <label class="field" style="max-width: 260px">Validade padrão do orçamento (dias)<input v-model.number="form.quoteValidityDays" type="number" min="1" max="365" /></label>
        <label class="field" style="max-width: 420px">Estoque baixo padrão (un.)<input v-model.number="form.lowStockDefault" type="number" min="0" max="100000" /><span class="small muted">Vale para produtos sem estoque mínimo próprio. 0 = avisar só quando zerar.</span></label>
        <h3 style="margin: 8px 0 0">Pix (QR Code com o valor, sem integração com banco)</h3>
        <div class="grid grid-2">
          <label class="field">Chave Pix (CPF, CNPJ, e-mail, telefone ou aleatória)<input v-model="form.pixKey" maxlength="80" placeholder="em branco = sem QR Code" /></label>
          <label class="field">Nome do recebedor (até 25 letras, sem acento)<input v-model="form.pixName" maxlength="25" placeholder="usa o nome fantasia" /></label>
          <label class="field">Cidade (até 15 letras, sem acento)<input v-model="form.pixCity" maxlength="15" placeholder="usa a cidade da loja" /></label>
          <label class="check" style="align-self: end"><input v-model="form.pixOnReceipt" type="checkbox" /> Imprimir QR Pix do valor a prazo no cupom</label>
        </div>
        <h3 style="margin: 8px 0 0">Segurança</h3>
        <div class="grid grid-2">
          <label class="field">Desconto máximo do operador sem gerente (%)<input v-model.number="form.operatorMaxDiscountPct" type="number" min="0" max="100" /></label>
          <label class="field">Bloquear a tela após (minutos sem uso, 0 = nunca)<input v-model.number="form.lockMinutes" type="number" min="0" max="240" /></label>
        </div>
        <label class="check"><input v-model="form.cashPerUser" type="checkbox" /> Caixa por usuário: cada um abre, usa e fecha o próprio caixa (só muda com todos os caixas fechados)</label>
        <h3 style="margin: 8px 0 0">Importação de NF-e: o que entra no custo do produto</h3>
        <div class="row" style="gap: 14px">
          <label class="check"><input v-model="form.nfeCostFreight" type="checkbox" /> Frete</label>
          <label class="check"><input v-model="form.nfeCostIpi" type="checkbox" /> IPI</label>
          <label class="check"><input v-model="form.nfeCostSt" type="checkbox" /> ICMS-ST</label>
          <label class="check"><input v-model="form.nfeCostOther" type="checkbox" /> Seguro e outras despesas</label>
          <label class="check"><input v-model="form.nfeCostDiscount" type="checkbox" /> Descontar o desconto</label>
        </div>
        <div v-if="settings.error" class="alert error">{{ settings.error }}</div>
        <div class="row end">
          <button class="primary" type="submit" :disabled="settings.busy">{{ settings.busy ? 'Salvando…' : 'Salvar' }}</button>
        </div>
      </form>
    </section>

    <BackgroundSettings v-if="can('alterar_configuracoes')" />
    <section v-if="can('alterar_configuracoes')" class="card stack">
      <h2>Impressão</h2>
      <label class="field">Papel
        <select v-model="form.paperWidth" form="settings-form">
          <option value="80">Térmica 80 mm</option>
          <option value="58">Térmica 58 mm</option>
          <option value="a4">A4 / impressora comum</option>
        </select>
      </label>
      <template v-if="form.paperWidth !== 'a4'">
        <div class="row">
          <label class="field" style="flex: 1">Impressora térmica
            <select v-model="form.printerName" form="settings-form">
              <option value="">Nenhuma (usar o diálogo de impressão do Windows)</option>
              <option v-for="p in printers" :key="p" :value="p">{{ p }}</option>
              <option v-if="form.printerName && !printers.includes(form.printerName)" :value="form.printerName">{{ form.printerName }} (não encontrada)</option>
            </select>
          </label>
          <button type="button" style="align-self: end" @click="loadPrinters">Atualizar lista</button>
        </div>
        <label class="check"><input v-model="form.cutPaper" type="checkbox" form="settings-form" /> Cortar o papel no fim do comprovante</label>
        <label class="check"><input v-model="form.openDrawer" type="checkbox" form="settings-form" /> Abrir a gaveta de dinheiro ao imprimir</label>
      </template>
      <label class="check"><input v-model="form.autoReceipt" type="checkbox" form="settings-form" /> Imprimir o comprovante após cada venda, recebimento a prazo e sangria/suprimento</label>
      <p class="small muted" style="margin: 0">
        Com uma térmica escolhida, o comprovante sai direto, sem abrir janela. A impressora precisa estar instalada no
        Windows (driver do fabricante ou "Generic / Text Only").
      </p>
      <div v-if="settings.error" class="alert error">{{ settings.error }}</div>
      <div class="row end">
        <button type="button" :disabled="settings.busy" @click="testPrint">Salvar e imprimir teste</button>
        <button class="primary" type="submit" form="settings-form" :disabled="settings.busy">Salvar</button>
      </div>
    </section>

    <section class="card stack">
      <h2>Backup e restauração</h2>
      <p class="muted">
        Faça backups com frequência e guarde cópias fora deste computador (pen drive, outro disco ou nuvem). O backup
        gera uma cópia íntegra do banco, inclusive das últimas vendas, e é verificado antes de ser salvo.
      </p>
      <div class="row">
        <button class="primary" :disabled="backupBusy" @click="backup">{{ backupBusy ? 'Aguarde…' : 'Criar backup' }}</button>
        <button v-if="can('restaurar_backup')" class="danger" :disabled="backupBusy" @click="confirmRestore = true">Restaurar backup…</button>
      </div>
      <div v-if="lastBackup" class="alert success">{{ lastBackup }}</div>
      <h3 style="margin: 8px 0 0">Backup automático</h3>
      <template v-if="can('alterar_configuracoes')">
        <label class="check"><input v-model="form.backupAuto" type="checkbox" form="settings-form" /> Fazer um backup por dia automaticamente (na pasta de dados)</label>
        <div class="grid grid-2">
          <label class="field">Guardar os últimos (backups)<input v-model.number="form.backupKeep" type="number" min="1" max="90" form="settings-form" /></label>
          <label class="field">Avisar se a cópia externa tiver mais de (dias)<input v-model.number="form.backupWarnDays" type="number" min="1" max="60" form="settings-form" /></label>
        </div>
        <label class="field">Pasta onde o backup automático é salvo (o banco de dados continua onde está)
          <div class="row" style="flex-wrap: nowrap">
            <input :value="form.backupDir || 'Pasta padrão (backups, dentro da pasta de dados)'" readonly style="flex: 1" aria-label="Pasta do backup automático" />
            <button type="button" @click="chooseFolder('backupDir')">Escolher pasta</button>
            <button v-if="form.backupDir" type="button" class="link" @click="form.backupDir = ''">Usar a padrão</button>
          </div>
        </label>
        <label class="field">Cópia fora deste computador (pendrive ou pasta do Google Drive/OneDrive)
          <div class="row" style="flex-wrap: nowrap">
            <input :value="form.backupExternalDir || 'Nenhuma pasta escolhida'" readonly style="flex: 1" aria-label="Pasta da cópia externa" />
            <button type="button" @click="chooseFolder('backupExternalDir')">Escolher pasta</button>
            <button v-if="form.backupExternalDir" type="button" class="link" style="color: var(--danger)" @click="form.backupExternalDir = ''">Remover</button>
          </div>
        </label>
      </template>
      <ul v-if="autoStatus" class="small" style="margin: 0">
        <li>Último backup automático: {{ autoStatus.lastAuto ? formatDateTime(autoStatus.lastAuto) : 'ainda não feito' }} — em {{ autoStatus.autoDir }}</li>
        <li v-if="autoStatus.externalDir">Última cópia externa: {{ autoStatus.lastExternal ? formatDateTime(autoStatus.lastExternal) : 'ainda não feita' }}</li>
      </ul>
      <div v-if="autoStatus?.warning" class="alert warn">{{ autoStatus.warning }}</div>
      <div class="row end">
        <button type="button" :disabled="backupBusy" @click="backupNow">{{ backupBusy ? 'Aguarde…' : can('alterar_configuracoes') ? 'Salvar e fazer backup agora' : 'Fazer backup agora' }}</button>
        <button v-if="can('alterar_configuracoes')" class="primary" type="submit" form="settings-form" :disabled="settings.busy">Salvar</button>
      </div>
      <div v-if="pending" class="alert warn stack">
        <span>
          <strong>Restauração agendada.</strong> O backup escolhido foi validado e será aplicado quando o programa for
          fechado e aberto novamente. Não registre novas vendas até lá: elas não estarão no backup.
        </span>
        <div class="row">
          <button class="primary" @click="QuitApp()">Fechar o programa agora</button>
          <button @click="cancelRestore">Cancelar restauração</button>
        </div>
      </div>
      <details>
        <summary>Orientações de recuperação</summary>
        <ul class="small">
          <li>Ao restaurar, o banco atual não é apagado: ele é guardado na pasta <code>backups</code>, dentro da pasta de dados.</li>
          <li>Se o arquivo de backup estiver danificado ou for de uma versão mais nova do programa, a restauração é recusada.</li>
          <li>Se o programa não abrir o banco, feche outros programas que possam estar usando o arquivo e verifique o espaço em disco; depois restaure o backup mais recente.</li>
          <li>Backup só protege o que foi copiado: se o disco do computador falhar e não houver cópia em outro lugar, os dados serão perdidos.</li>
        </ul>
      </details>
    </section>

    <section class="card stack">
      <h2>Sobre e diagnóstico</h2>
      <dl v-if="info" class="small" style="margin: 0">
        <dt class="muted">Versão do aplicativo</dt><dd>{{ info.version }}</dd>
        <dt class="muted">Versão do banco de dados</dt><dd>{{ info.schemaVersion || '—' }}</dd>
        <dt class="muted">Pasta de dados</dt><dd style="word-break: break-all">{{ info.dataDir || '—' }}</dd>
        <dt class="muted">Arquivo do banco</dt><dd style="word-break: break-all">{{ info.databasePath || '—' }}</dd>
      </dl>
      <p class="small muted">
        Erros e falhas do sistema ficam registrados no arquivo <code>pdv.log</code>, na subpasta <code>logs</code> da pasta de
        dados. Se algo der errado, envie esse arquivo ao suporte. Os dados não são criptografados: qualquer pessoa com acesso a
        este usuário do Windows pode ler o arquivo do banco.
      </p>
      <div class="row"><button type="button" @click="openLogs">Abrir pasta de registros de erro</button></div>
      <div class="alert info small">
        Este sistema não emite documentos fiscais (NFC-e, NF-e, SAT) e não se integra a bancos ou maquininhas de cartão.
        Pix e cartão são apenas registrados pelo operador.
      </div>
    </section>
  </div>

  <ReceiptModal v-if="testOpen" kind="teste" :id="0" @close="testOpen = false" />
  <ConfirmDialog
    v-if="confirmRestore"
    title="Restaurar backup"
    message="Você vai escolher um arquivo de backup. Ele será verificado e aplicado na próxima abertura do programa, substituindo os dados atuais (que ficarão guardados na pasta de backups). Vendas feitas depois do backup deixarão de aparecer."
    confirm-label="Escolher arquivo"
    danger
    @confirm="restore"
    @cancel="confirmRestore = false"
  />
</template>

<style scoped>
.msgs { border: 1px solid var(--border); border-radius: var(--radius); padding: 10px 12px; }
.msgs summary { cursor: pointer; }
.msgs[open] summary { margin-bottom: 8px; }
</style>
