// Caminho: catraca/frontend/src/router/index.ts

import { createRouter, createWebHashHistory } from 'vue-router'
import { can } from '../composables/session'

// Hash history: o Wails serve os arquivos embutidos, sem servidor para
// reescrever rotas.
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/inicio' },
    { path: '/inicio', component: () => import('../views/Dashboard.vue'), meta: { title: 'Painel' } },
    { path: '/vendas', component: () => import('../views/Vendas.vue'), meta: { title: 'Vendas', fill: true } },
    { path: '/devolucoes', component: () => import('../views/Devolucoes.vue'), meta: { title: 'Trocas e devoluções' } },
    { path: '/produtos', component: () => import('../views/Produtos.vue'), meta: { title: 'Produtos', mode: 'produtos' } },
    { path: '/estoque', component: () => import('../views/Produtos.vue'), meta: { title: 'Estoque', mode: 'estoque' } },
    { path: '/clientes', component: () => import('../views/Clientes.vue'), meta: { title: 'Clientes' } },
    { path: '/fiado', component: () => import('../views/Fiado.vue'), meta: { title: 'A prazo' } },
    // A lista de assinaturas fica dentro de Alunos e rostos (Mensalidades e cobrança).
    { path: '/assinaturas', redirect: '/catraca/alunos?aba=mensalidades' },
    { path: '/catraca', component: () => import('../views/Catraca.vue'), meta: { title: 'Catraca' } },
    { path: '/catraca/entradas', component: () => import('../views/CatracaEntradas.vue'), meta: { title: 'Entradas' } },
    { path: '/catraca/alunos', component: () => import('../views/CatracaAlunos.vue'), meta: { title: 'Alunos e rostos' } },
    { path: '/catraca/funcionarios', component: () => import('../views/CatracaFuncionarios.vue'), meta: { title: 'Funcionários', perms: ['catraca_funcionarios'] } },
    { path: '/catraca/config', component: () => import('../views/CatracaConfig.vue'), meta: { title: 'Configurar catraca', perms: ['catraca_configurar'] } },
    { path: '/comandas', component: () => import('../views/Comandas.vue'), meta: { title: 'Comandas' } },
    { path: '/caixa', component: () => import('../views/Caixa.vue'), meta: { title: 'Caixa' } },
    { path: '/historico', component: () => import('../views/HistoricoVendas.vue'), meta: { title: 'Histórico de vendas' } },
    { path: '/relatorios', component: () => import('../views/Relatorios.vue'), meta: { title: 'Relatórios' } },
    { path: '/relatorios/produtos', component: () => import('../views/RelatoriosAnalise.vue'), meta: { title: 'Relatórios', section: 'produtos' } },
    { path: '/financeiro/contas', component: () => import('../views/ContasPagar.vue'), meta: { title: 'Contas a pagar', perms: ['contas_a_pagar'] } },
    { path: '/financeiro/lucro', component: () => import('../views/RelatoriosAnalise.vue'), meta: { title: 'Lucro', section: 'lucro', perms: ['ver_custo'] } },
    { path: '/financeiro/descontos', component: () => import('../views/RelatoriosAnalise.vue'), meta: { title: 'Descontos', section: 'descontos' } },
    { path: '/configuracoes', component: () => import('../views/Configuracoes.vue'), meta: { title: 'Configurações', perms: ['alterar_configuracoes', 'restaurar_backup'] } },
    { path: '/orcamentos', component: () => import('../views/Orcamentos.vue'), meta: { title: 'Orçamentos' } },
    { path: '/promocoes', component: () => import('../views/Promocoes.vue'), meta: { title: 'Promoções', perms: ['alterar_preco'] } },
    { path: '/compras', component: () => import('../views/Compras.vue'), meta: { title: 'Entrada de mercadoria', perms: ['compras'] } },
    { path: '/fornecedores', component: () => import('../views/Fornecedores.vue'), meta: { title: 'Fornecedores', perms: ['compras'] } },
    { path: '/apagar-testes', component: () => import('../views/ApagarTestes.vue'), meta: { title: 'Apagar vendas de teste', perms: ['apagar_testes'] } },
    { path: '/usuarios', component: () => import('../views/Usuarios.vue'), meta: { title: 'Usuários', perms: ['gerenciar_usuarios'] } },
    { path: '/etiquetas', component: () => import('../views/Etiquetas.vue'), meta: { title: 'Etiquetas' } },
    { path: '/assistente', component: () => import('../views/Assistente.vue'), meta: { title: 'Primeira configuração', perms: ['alterar_configuracoes'] } },
    { path: '/licenca', component: () => import('../views/Licenca.vue'), meta: { title: 'Licença' } },
    { path: '/auditoria', component: () => import('../views/Auditoria.vue'), meta: { title: 'Auditoria', perms: ['ver_auditoria'] } },
  ],
})

// Tela sem permissão (qualquer uma das listadas basta) volta para Vendas.
router.beforeEach((to) => {
  const perms = to.meta.perms as string[] | undefined
  if (perms && !perms.some(can)) return '/vendas'
})
