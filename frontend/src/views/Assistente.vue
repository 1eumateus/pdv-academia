<!-- Caminho: pdv/frontend/src/views/Assistente.vue -->
<script setup lang="ts">
// Assistente de primeira configuração: loja → impressora → produtos → licença.
// Cada passo grava pelo mesmo SaveSettings das Configurações (o backend valida).
// "Pular" marca como concluído; dá para abrir de novo em Configurações.
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ActivateLicense, GetSettings, ListPrinters, SaveSettings } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import ImportModal from '../components/ImportModal.vue'
import { licenseStatus, refreshLicense, refreshSettings } from '../composables/cashState'
import { currentUser } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Settings } from '../types'

const router = useRouter()
const toast = useToast()
const steps = ['Boas-vindas', 'Dados da loja', 'Impressora', 'Produtos', 'Licença']
const step = ref(0)
const form = reactive<Settings>(models.Settings.createFrom({}))
const printers = ref<string[]>([])
const busy = ref(false)
const error = ref('')
const importOpen = ref(false)
const imported = ref(false)
const licenseText = ref('')

onMounted(async () => {
  try {
    Object.assign(form, await call(GetSettings()))
    printers.value = await call(ListPrinters())
  } catch (e) {
    error.value = errorMessage(e)
  }
  refreshLicense()
})

async function save(extra: Partial<Settings> = {}): Promise<boolean> {
  busy.value = true
  error.value = ''
  try {
    Object.assign(form, await call(SaveSettings(models.Settings.createFrom({ ...form, ...extra }))))
    await refreshSettings()
    return true
  } catch (e) {
    error.value = errorMessage(e)
    return false
  } finally {
    busy.value = false
  }
}

async function next() {
  if ((step.value === 1 || step.value === 2) && !(await save())) return
  step.value++
}

async function activate() {
  busy.value = true
  error.value = ''
  try {
    licenseStatus.value = await call(ActivateLicense(licenseText.value))
    licenseText.value = ''
    toast.success('Licença ativada.')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function finish() {
  if (await save({ setupDone: true })) {
    toast.success('Tudo pronto! Bom trabalho.')
    router.replace('/vendas')
  }
}
const isLast = computed(() => step.value === steps.length - 1)
</script>

<template>
  <div class="stack" style="max-width: 760px; margin: 0 auto">
    <ol class="wizard-steps" aria-label="Passos">
      <li v-for="(s, i) in steps" :key="s" :class="{ done: i < step, current: i === step }">{{ i + 1 }}. {{ s }}</li>
    </ol>

    <section class="card stack">
      <template v-if="step === 0">
        <h2>Bem-vindo{{ currentUser ? `, ${currentUser.name.split(' ')[0]}` : '' }}!</h2>
        <p>Seu usuário de administrador já foi criado. Em poucos passos o sistema fica pronto para vender:</p>
        <ul>
          <li>dados da loja (aparecem no cupom e no orçamento);</li>
          <li>impressora e tamanho do papel;</li>
          <li>produtos (importe de uma planilha, se já tiver);</li>
          <li>licença ou período de teste de 15 dias.</li>
        </ul>
        <p class="small muted">Dá para pular agora e continuar depois em Administração → Configurações.</p>
      </template>

      <template v-else-if="step === 1">
        <h2>Dados da loja</h2>
        <div class="grid grid-2">
          <label class="field">Nome fantasia *<input v-model="form.storeName" maxlength="200" required /></label>
          <label class="field">CNPJ ou CPF<input v-model="form.document" maxlength="20" /></label>
          <label class="field">Telefone<input v-model="form.phone" maxlength="30" inputmode="tel" /></label>
          <label class="field">WhatsApp<input v-model="form.whatsapp" maxlength="30" inputmode="tel" /></label>
          <label class="field">Rua<input v-model="form.street" maxlength="200" /></label>
          <label class="field">Número<input v-model="form.number" maxlength="20" /></label>
          <label class="field">Bairro<input v-model="form.district" maxlength="200" /></label>
          <label class="field">Cidade<input v-model="form.city" maxlength="200" /></label>
          <label class="field">UF<input v-model="form.state" maxlength="2" placeholder="SP" /></label>
          <label class="field">CEP<input v-model="form.zipCode" maxlength="10" inputmode="numeric" /></label>
        </div>
        <p class="small muted" style="margin: 0">Logo, Pix e os demais dados ficam em Configurações.</p>
      </template>

      <template v-else-if="step === 2">
        <h2>Impressora e papel</h2>
        <label class="field">Tamanho do papel
          <select v-model="form.paperWidth">
            <option value="80">Térmica 80 mm</option><option value="58">Térmica 58 mm</option><option value="a4">Folha A4 (impressora comum)</option>
          </select>
        </label>
        <label v-if="form.paperWidth !== 'a4'" class="field">Impressora térmica
          <select v-model="form.printerName">
            <option value="">Escolher na hora de imprimir</option>
            <option v-for="p in printers" :key="p" :value="p">{{ p }}</option>
          </select>
        </label>
        <label class="check"><input v-model="form.autoReceipt" type="checkbox" /> Imprimir o comprovante após cada venda</label>
        <p class="small muted" style="margin: 0">Para testar a impressão, use "Salvar e imprimir teste" em Configurações.</p>
      </template>

      <template v-else-if="step === 3">
        <h2>Produtos</h2>
        <p>Se você já tem os produtos numa planilha (Excel ou CSV), importe agora. Se não, cadastre depois em Produtos.</p>
        <div class="row">
          <button class="primary" @click="importOpen = true">Importar planilha de produtos</button>
        </div>
        <div v-if="imported" class="alert success" style="margin: 0">Produtos importados.</div>
      </template>

      <template v-else>
        <h2>Licença</h2>
        <p v-if="licenseStatus">{{ licenseStatus.message }}</p>
        <p class="small">ID deste computador (envie ao suporte para receber a licença): <strong class="mono">{{ licenseStatus?.machineId }}</strong></p>
        <label class="field">Já tem a licença? Cole aqui
          <textarea v-model="licenseText" rows="3" class="mono" spellcheck="false" placeholder="PDV1.…" />
        </label>
        <div class="row end"><button :disabled="busy || !licenseText.trim()" @click="activate">Ativar licença</button></div>
      </template>

      <div v-if="error" class="alert error" role="alert">{{ error }}</div>
      <div class="row between">
        <button class="link" :disabled="busy" @click="finish">Pular assistente</button>
        <span class="row">
          <button v-if="step > 0" :disabled="busy" @click="step--">Voltar</button>
          <button v-if="!isLast" class="primary" :disabled="busy" @click="next">{{ step === 0 ? 'Começar' : 'Continuar' }}</button>
          <button v-else class="success" :disabled="busy" @click="finish">{{ licenseStatus?.state === 'ativa' ? 'Concluir' : 'Concluir e usar o teste' }}</button>
        </span>
      </div>
    </section>
  </div>
  <ImportModal v-if="importOpen" kind="produtos" @close="importOpen = false" @done="imported = true" />
</template>

<style scoped>
.wizard-steps { display: flex; flex-wrap: wrap; gap: 6px 16px; list-style: none; padding: 0; margin: 0; color: var(--muted); font-size: 14px; }
.wizard-steps .current { color: var(--primary); font-weight: 700; }
.wizard-steps .done { color: var(--success); }
</style>
