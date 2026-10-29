export namespace models {
	
	export class ABCRow {
	    productId: number;
	    name: string;
	    qty: number;
	    revenueCents: number;
	    sharePct: number;
	    cumulativePct: number;
	    class: string;
	
	    static createFrom(source: any = {}) {
	        return new ABCRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.name = source["name"];
	        this.qty = source["qty"];
	        this.revenueCents = source["revenueCents"];
	        this.sharePct = source["sharePct"];
	        this.cumulativePct = source["cumulativePct"];
	        this.class = source["class"];
	    }
	}
	export class ABCReport {
	    rows: ABCRow[];
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new ABCReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], ABCRow);
	        this.totalCents = source["totalCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class AccessRules {
	    weekdays: number[];
	    weeklyLimit: number;
	    dailyLimit: number;
	
	    static createFrom(source: any = {}) {
	        return new AccessRules(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekdays = source["weekdays"];
	        this.weeklyLimit = source["weeklyLimit"];
	        this.dailyLimit = source["dailyLimit"];
	    }
	}
	export class AgingCustomer {
	    customerId: number;
	    name: string;
	    phone?: string;
	    balanceCents: number;
	    upto30Cents: number;
	    upto60Cents: number;
	    upto90Cents: number;
	    over90Cents: number;
	    oldestDays: number;
	
	    static createFrom(source: any = {}) {
	        return new AgingCustomer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customerId = source["customerId"];
	        this.name = source["name"];
	        this.phone = source["phone"];
	        this.balanceCents = source["balanceCents"];
	        this.upto30Cents = source["upto30Cents"];
	        this.upto60Cents = source["upto60Cents"];
	        this.upto90Cents = source["upto90Cents"];
	        this.over90Cents = source["over90Cents"];
	        this.oldestDays = source["oldestDays"];
	    }
	}
	export class AppInfo {
	    version: string;
	    dataDir: string;
	    databasePath: string;
	    schemaVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.databasePath = source["databasePath"];
	        this.schemaVersion = source["schemaVersion"];
	    }
	}
	export class AuditEntry {
	    id: number;
	    userName?: string;
	    authorizedBy?: string;
	    action: string;
	    entity?: string;
	    entityId?: number;
	    beforeJson?: string;
	    afterJson?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new AuditEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.userName = source["userName"];
	        this.authorizedBy = source["authorizedBy"];
	        this.action = source["action"];
	        this.entity = source["entity"];
	        this.entityId = source["entityId"];
	        this.beforeJson = source["beforeJson"];
	        this.afterJson = source["afterJson"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Backgrounds {
	    login: string;
	    app: string;
	
	    static createFrom(source: any = {}) {
	        return new Backgrounds(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.app = source["app"];
	    }
	}
	export class BackupResult {
	    path: string;
	    sizeBytes: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new BackupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BackupStatus {
	    autoEnabled: boolean;
	    autoDir: string;
	    autoDirError: string;
	    // Go type: time
	    lastAuto?: any;
	    externalDir: string;
	    // Go type: time
	    lastExternal?: any;
	    externalError: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoEnabled = source["autoEnabled"];
	        this.autoDir = source["autoDir"];
	        this.autoDirError = source["autoDirError"];
	        this.lastAuto = this.convertValues(source["lastAuto"], null);
	        this.externalDir = source["externalDir"];
	        this.lastExternal = this.convertValues(source["lastExternal"], null);
	        this.externalError = source["externalError"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Bill {
	    id: number;
	    description: string;
	    category: string;
	    payee: string;
	    amountCents: number;
	    dueDate: string;
	    barcode: string;
	    notes: string;
	    recurring: boolean;
	    status: string;
	    paidDate?: string;
	    paidCents?: number;
	    payMethod?: string;
	    fromCash: boolean;
	    boletoFiles: number;
	    receiptFiles: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Bill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.payee = source["payee"];
	        this.amountCents = source["amountCents"];
	        this.dueDate = source["dueDate"];
	        this.barcode = source["barcode"];
	        this.notes = source["notes"];
	        this.recurring = source["recurring"];
	        this.status = source["status"];
	        this.paidDate = source["paidDate"];
	        this.paidCents = source["paidCents"];
	        this.payMethod = source["payMethod"];
	        this.fromCash = source["fromCash"];
	        this.boletoFiles = source["boletoFiles"];
	        this.receiptFiles = source["receiptFiles"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BillCategoryTotal {
	    category: string;
	    count: number;
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new BillCategoryTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	    }
	}
	export class BillFile {
	    id: number;
	    kind: string;
	    name: string;
	    mime: string;
	    size: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new BillFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BillInput {
	    id: number;
	    description: string;
	    category: string;
	    payee: string;
	    amountCents: number;
	    dueDate: string;
	    barcode: string;
	    notes: string;
	    recurring: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BillInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.payee = source["payee"];
	        this.amountCents = source["amountCents"];
	        this.dueDate = source["dueDate"];
	        this.barcode = source["barcode"];
	        this.notes = source["notes"];
	        this.recurring = source["recurring"];
	    }
	}
	export class CashMovement {
	    id: number;
	    sessionId: number;
	    type: string;
	    paymentMethod: string;
	    amountCents: number;
	    description?: string;
	    referenceType?: string;
	    referenceId?: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new CashMovement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.type = source["type"];
	        this.paymentMethod = source["paymentMethod"];
	        this.amountCents = source["amountCents"];
	        this.description = source["description"];
	        this.referenceType = source["referenceType"];
	        this.referenceId = source["referenceId"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CashReportTotal {
	    method: string;
	    inCents: number;
	    outCents: number;
	
	    static createFrom(source: any = {}) {
	        return new CashReportTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.method = source["method"];
	        this.inCents = source["inCents"];
	        this.outCents = source["outCents"];
	    }
	}
	export class CashReportRow {
	    receipt: string;
	    // Go type: time
	    at: any;
	    history: string;
	    detail: string;
	    amountCents: number;
	    method: string;
	    in: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CashReportRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.receipt = source["receipt"];
	        this.at = this.convertValues(source["at"], null);
	        this.history = source["history"];
	        this.detail = source["detail"];
	        this.amountCents = source["amountCents"];
	        this.method = source["method"];
	        this.in = source["in"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CashReport {
	    startDate: string;
	    endDate: string;
	    userName?: string;
	    rows: CashReportRow[];
	    totals: CashReportTotal[];
	    inCents: number;
	    outCents: number;
	
	    static createFrom(source: any = {}) {
	        return new CashReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.userName = source["userName"];
	        this.rows = this.convertValues(source["rows"], CashReportRow);
	        this.totals = this.convertValues(source["totals"], CashReportTotal);
	        this.inCents = source["inCents"];
	        this.outCents = source["outCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class CashSession {
	    id: number;
	    status: string;
	    openingCents: number;
	    expectedCents?: number;
	    countedCents?: number;
	    differenceCents?: number;
	    notes?: string;
	    // Go type: time
	    openedAt: any;
	    // Go type: time
	    closedAt?: any;
	    openedByName?: string;
	    closedByName?: string;
	    ownerUserId?: number;
	
	    static createFrom(source: any = {}) {
	        return new CashSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.openingCents = source["openingCents"];
	        this.expectedCents = source["expectedCents"];
	        this.countedCents = source["countedCents"];
	        this.differenceCents = source["differenceCents"];
	        this.notes = source["notes"];
	        this.openedAt = this.convertValues(source["openedAt"], null);
	        this.closedAt = this.convertValues(source["closedAt"], null);
	        this.openedByName = source["openedByName"];
	        this.closedByName = source["closedByName"];
	        this.ownerUserId = source["ownerUserId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PaymentTotal {
	    paymentMethod: string;
	    count: number;
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new PaymentTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paymentMethod = source["paymentMethod"];
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	    }
	}
	export class Sale {
	    id: number;
	    customerId?: number;
	    customerName?: string;
	    comandaId?: number;
	    cashSessionId?: number;
	    subtotalCents: number;
	    discountCents: number;
	    totalCents: number;
	    paymentMethod: string;
	    status: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    cancelledAt?: any;
	    cancellationReason?: string;
	    userId?: number;
	    userName?: string;
	    cancelledByName?: string;
	    itemsLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new Sale(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.comandaId = source["comandaId"];
	        this.cashSessionId = source["cashSessionId"];
	        this.subtotalCents = source["subtotalCents"];
	        this.discountCents = source["discountCents"];
	        this.totalCents = source["totalCents"];
	        this.paymentMethod = source["paymentMethod"];
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.cancelledAt = this.convertValues(source["cancelledAt"], null);
	        this.cancellationReason = source["cancellationReason"];
	        this.userId = source["userId"];
	        this.userName = source["userName"];
	        this.cancelledByName = source["cancelledByName"];
	        this.itemsLabel = source["itemsLabel"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CashSummary {
	    session: CashSession;
	    movements: CashMovement[];
	    sales: Sale[];
	    byPaymentMethod: PaymentTotal[];
	    expectedCashCents: number;
	
	    static createFrom(source: any = {}) {
	        return new CashSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = this.convertValues(source["session"], CashSession);
	        this.movements = this.convertValues(source["movements"], CashMovement);
	        this.sales = this.convertValues(source["sales"], Sale);
	        this.byPaymentMethod = this.convertValues(source["byPaymentMethod"], PaymentTotal);
	        this.expectedCashCents = source["expectedCashCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Category {
	    id: number;
	    name: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.active = source["active"];
	    }
	}
	export class CategoryTotal {
	    categoryName: string;
	    qty: number;
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new CategoryTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categoryName = source["categoryName"];
	        this.qty = source["qty"];
	        this.totalCents = source["totalCents"];
	    }
	}
	export class PaymentInput {
	    method: string;
	    amountCents: number;
	    receivedCents: number;
	    cardBrand: string;
	    installments: number;
	
	    static createFrom(source: any = {}) {
	        return new PaymentInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.method = source["method"];
	        this.amountCents = source["amountCents"];
	        this.receivedCents = source["receivedCents"];
	        this.cardBrand = source["cardBrand"];
	        this.installments = source["installments"];
	    }
	}
	export class CloseComandaInput {
	    comandaId: number;
	    paymentMethod: string;
	    customerId?: number;
	    discountCents: number;
	    payments: PaymentInput[];
	
	    static createFrom(source: any = {}) {
	        return new CloseComandaInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comandaId = source["comandaId"];
	        this.paymentMethod = source["paymentMethod"];
	        this.customerId = source["customerId"];
	        this.discountCents = source["discountCents"];
	        this.payments = this.convertValues(source["payments"], PaymentInput);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Comanda {
	    id: number;
	    tableNo: string;
	    status: string;
	    // Go type: time
	    openedAt: any;
	    // Go type: time
	    closedAt?: any;
	    totalCents: number;
	    itemCount: number;
	    saleId?: number;
	
	    static createFrom(source: any = {}) {
	        return new Comanda(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tableNo = source["tableNo"];
	        this.status = source["status"];
	        this.openedAt = this.convertValues(source["openedAt"], null);
	        this.closedAt = this.convertValues(source["closedAt"], null);
	        this.totalCents = source["totalCents"];
	        this.itemCount = source["itemCount"];
	        this.saleId = source["saleId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ComandaItem {
	    id: number;
	    comandaId: number;
	    productId: number;
	    productNameSnapshot: string;
	    qty: number;
	    unitPriceCents: number;
	    subtotalCents: number;
	    productActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ComandaItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.comandaId = source["comandaId"];
	        this.productId = source["productId"];
	        this.productNameSnapshot = source["productNameSnapshot"];
	        this.qty = source["qty"];
	        this.unitPriceCents = source["unitPriceCents"];
	        this.subtotalCents = source["subtotalCents"];
	        this.productActive = source["productActive"];
	    }
	}
	export class ComandaDetails {
	    comanda: Comanda;
	    items: ComandaItem[];
	
	    static createFrom(source: any = {}) {
	        return new ComandaDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comanda = this.convertValues(source["comanda"], Comanda);
	        this.items = this.convertValues(source["items"], ComandaItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Delivery {
	    address: string;
	    phone: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new Delivery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.phone = source["phone"];
	        this.notes = source["notes"];
	    }
	}
	export class SaleItemInput {
	    productId: number;
	    qty: number;
	    discountCents: number;
	
	    static createFrom(source: any = {}) {
	        return new SaleItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.qty = source["qty"];
	        this.discountCents = source["discountCents"];
	    }
	}
	export class CreateSaleInput {
	    items: SaleItemInput[];
	    paymentMethod: string;
	    customerId?: number;
	    discountCents: number;
	    payments: PaymentInput[];
	    requestId: string;
	    delivery?: Delivery;
	    quoteId?: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateSaleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], SaleItemInput);
	        this.paymentMethod = source["paymentMethod"];
	        this.customerId = source["customerId"];
	        this.discountCents = source["discountCents"];
	        this.payments = this.convertValues(source["payments"], PaymentInput);
	        this.requestId = source["requestId"];
	        this.delivery = this.convertValues(source["delivery"], Delivery);
	        this.quoteId = source["quoteId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreditPayment {
	    id: number;
	    customerId: number;
	    customerName: string;
	    amountCents: number;
	    paymentMethod: string;
	    kind: string;
	    reversesPaymentId?: number;
	    reversed: boolean;
	    // Go type: time
	    createdAt: any;
	    notes?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreditPayment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.amountCents = source["amountCents"];
	        this.paymentMethod = source["paymentMethod"];
	        this.kind = source["kind"];
	        this.reversesPaymentId = source["reversesPaymentId"];
	        this.reversed = source["reversed"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.notes = source["notes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Customer {
	    id: number;
	    name: string;
	    phone?: string;
	    document?: string;
	    email?: string;
	    zipCode?: string;
	    street?: string;
	    number?: string;
	    complement?: string;
	    district?: string;
	    city?: string;
	    state?: string;
	    reference?: string;
	    notes?: string;
	    creditLimitCents: number;
	    creditBalanceCents: number;
	    storeCreditCents: number;
	    active: boolean;
	    // Go type: time
	    lastChargeAt?: any;
	    birthDate?: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Customer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.phone = source["phone"];
	        this.document = source["document"];
	        this.email = source["email"];
	        this.zipCode = source["zipCode"];
	        this.street = source["street"];
	        this.number = source["number"];
	        this.complement = source["complement"];
	        this.district = source["district"];
	        this.city = source["city"];
	        this.state = source["state"];
	        this.reference = source["reference"];
	        this.notes = source["notes"];
	        this.creditLimitCents = source["creditLimitCents"];
	        this.creditBalanceCents = source["creditBalanceCents"];
	        this.storeCreditCents = source["storeCreditCents"];
	        this.active = source["active"];
	        this.lastChargeAt = this.convertValues(source["lastChargeAt"], null);
	        this.birthDate = source["birthDate"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FiadoNote {
	    saleId: number;
	    // Go type: time
	    createdAt: any;
	    description: string;
	    amountCents: number;
	    paidCents: number;
	    openCents: number;
	    status: string;
	    daysOpen: number;
	
	    static createFrom(source: any = {}) {
	        return new FiadoNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleId = source["saleId"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.description = source["description"];
	        this.amountCents = source["amountCents"];
	        this.paidCents = source["paidCents"];
	        this.openCents = source["openCents"];
	        this.status = source["status"];
	        this.daysOpen = source["daysOpen"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomerHistory {
	    customer: Customer;
	    sales: Sale[];
	    payments: CreditPayment[];
	    notes: FiadoNote[];
	
	    static createFrom(source: any = {}) {
	        return new CustomerHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customer = this.convertValues(source["customer"], Customer);
	        this.sales = this.convertValues(source["sales"], Sale);
	        this.payments = this.convertValues(source["payments"], CreditPayment);
	        this.notes = this.convertValues(source["notes"], FiadoNote);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomerInput {
	    id: number;
	    name: string;
	    phone?: string;
	    document?: string;
	    email?: string;
	    zipCode?: string;
	    street?: string;
	    number?: string;
	    complement?: string;
	    district?: string;
	    city?: string;
	    state?: string;
	    reference?: string;
	    notes?: string;
	    creditLimitCents: number;
	    active: boolean;
	    birthDate?: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomerInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.phone = source["phone"];
	        this.document = source["document"];
	        this.email = source["email"];
	        this.zipCode = source["zipCode"];
	        this.street = source["street"];
	        this.number = source["number"];
	        this.complement = source["complement"];
	        this.district = source["district"];
	        this.city = source["city"];
	        this.state = source["state"];
	        this.reference = source["reference"];
	        this.notes = source["notes"];
	        this.creditLimitCents = source["creditLimitCents"];
	        this.active = source["active"];
	        this.birthDate = source["birthDate"];
	    }
	}
	export class StockAlert {
	    productId: number;
	    name: string;
	    stockQty: number;
	    minStock: number;
	
	    static createFrom(source: any = {}) {
	        return new StockAlert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.name = source["name"];
	        this.stockQty = source["stockQty"];
	        this.minStock = source["minStock"];
	    }
	}
	export class ProductSold {
	    productId: number;
	    name: string;
	    qty: number;
	    totalCents: number;
	    stockQty: number;
	    minStock: number;
	
	    static createFrom(source: any = {}) {
	        return new ProductSold(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.name = source["name"];
	        this.qty = source["qty"];
	        this.totalCents = source["totalCents"];
	        this.stockQty = source["stockQty"];
	        this.minStock = source["minStock"];
	    }
	}
	export class DayTotal {
	    date: string;
	    count: number;
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new DayTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	    }
	}
	export class PeriodTotals {
	    count: number;
	    totalCents: number;
	    avgCents: number;
	
	    static createFrom(source: any = {}) {
	        return new PeriodTotals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	        this.avgCents = source["avgCents"];
	    }
	}
	export class Dashboard {
	    allSales: boolean;
	    today: PeriodTotals;
	    yesterday: PeriodTotals;
	    month: PeriodTotals;
	    prevMonth: PeriodTotals;
	    days: DayTotal[];
	    topProducts: ProductSold[];
	    stockAlerts: StockAlert[];
	    lowStockCount: number;
	    zeroStockCount: number;
	    creditOpenCents: number;
	    creditCustomers: number;
	    openQuotes: number;
	    openQuotesCents: number;
	    cash?: CashSession;
	    cashExpectedCents: number;
	    billsDueToday: number;
	    billsOverdue: number;
	
	    static createFrom(source: any = {}) {
	        return new Dashboard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allSales = source["allSales"];
	        this.today = this.convertValues(source["today"], PeriodTotals);
	        this.yesterday = this.convertValues(source["yesterday"], PeriodTotals);
	        this.month = this.convertValues(source["month"], PeriodTotals);
	        this.prevMonth = this.convertValues(source["prevMonth"], PeriodTotals);
	        this.days = this.convertValues(source["days"], DayTotal);
	        this.topProducts = this.convertValues(source["topProducts"], ProductSold);
	        this.stockAlerts = this.convertValues(source["stockAlerts"], StockAlert);
	        this.lowStockCount = source["lowStockCount"];
	        this.zeroStockCount = source["zeroStockCount"];
	        this.creditOpenCents = source["creditOpenCents"];
	        this.creditCustomers = source["creditCustomers"];
	        this.openQuotes = source["openQuotes"];
	        this.openQuotesCents = source["openQuotesCents"];
	        this.cash = this.convertValues(source["cash"], CashSession);
	        this.cashExpectedCents = source["cashExpectedCents"];
	        this.billsDueToday = source["billsDueToday"];
	        this.billsOverdue = source["billsOverdue"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class DiscountByOperator {
	    userName: string;
	    sales: number;
	    salesWithDiscount: number;
	    subtotalCents: number;
	    discountCents: number;
	    promoCents: number;
	
	    static createFrom(source: any = {}) {
	        return new DiscountByOperator(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userName = source["userName"];
	        this.sales = source["sales"];
	        this.salesWithDiscount = source["salesWithDiscount"];
	        this.subtotalCents = source["subtotalCents"];
	        this.discountCents = source["discountCents"];
	        this.promoCents = source["promoCents"];
	    }
	}
	export class ExportColumn {
	    label: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.kind = source["kind"];
	    }
	}
	export class ExportTable {
	    title: string;
	    subtitle: string;
	    columns: ExportColumn[];
	    rows: string[][];
	
	    static createFrom(source: any = {}) {
	        return new ExportTable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.subtitle = source["subtitle"];
	        this.columns = this.convertValues(source["columns"], ExportColumn);
	        this.rows = source["rows"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class GateCheck {
	    allowed: boolean;
	    reason: string;
	    released: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GateCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowed = source["allowed"];
	        this.reason = source["reason"];
	        this.released = source["released"];
	    }
	}
	export class GateConfig {
	    relay: string;
	    serverPort: number;
	    toleranceDays: number;
	    deviceHost: string;
	    deviceLogin: string;
	    devicePassword: string;
	    secBoxId: number;
	    serialPort: string;
	    serialSignal: string;
	    pulseMs: number;
	
	    static createFrom(source: any = {}) {
	        return new GateConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relay = source["relay"];
	        this.serverPort = source["serverPort"];
	        this.toleranceDays = source["toleranceDays"];
	        this.deviceHost = source["deviceHost"];
	        this.deviceLogin = source["deviceLogin"];
	        this.devicePassword = source["devicePassword"];
	        this.secBoxId = source["secBoxId"];
	        this.serialPort = source["serialPort"];
	        this.serialSignal = source["serialSignal"];
	        this.pulseMs = source["pulseMs"];
	    }
	}
	export class GateEvent {
	    id: number;
	    // Go type: time
	    at: any;
	    customerId?: number;
	    name: string;
	    allowed: boolean;
	    reason: string;
	    source: string;
	    userName: string;
	
	    static createFrom(source: any = {}) {
	        return new GateEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = this.convertValues(source["at"], null);
	        this.customerId = source["customerId"];
	        this.name = source["name"];
	        this.allowed = source["allowed"];
	        this.reason = source["reason"];
	        this.source = source["source"];
	        this.userName = source["userName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GateImport {
	    deviceUsers: number;
	    linked: number;
	    created: number;
	    already: number;
	
	    static createFrom(source: any = {}) {
	        return new GateImport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceUsers = source["deviceUsers"];
	        this.linked = source["linked"];
	        this.created = source["created"];
	        this.already = source["already"];
	    }
	}
	export class GatePlan {
	    name: string;
	    rules: AccessRules;
	    dueDate: string;
	    paused: boolean;
	    everPaid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GatePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rules = this.convertValues(source["rules"], AccessRules);
	        this.dueDate = source["dueDate"];
	        this.paused = source["paused"];
	        this.everPaid = source["everPaid"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GateMember {
	    customerId: number;
	    name: string;
	    phone?: string;
	    active: boolean;
	    deviceUserId?: number;
	    blocked: boolean;
	    freeUntil: string;
	    notes: string;
	    plans: GatePlan[];
	    staff: boolean;
	    role: string;
	    daysThisWeek: number;
	    entriesToday: number;
	    allowed: boolean;
	    reason: string;
	    // Go type: time
	    lastAccess?: any;
	
	    static createFrom(source: any = {}) {
	        return new GateMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customerId = source["customerId"];
	        this.name = source["name"];
	        this.phone = source["phone"];
	        this.active = source["active"];
	        this.deviceUserId = source["deviceUserId"];
	        this.blocked = source["blocked"];
	        this.freeUntil = source["freeUntil"];
	        this.notes = source["notes"];
	        this.plans = this.convertValues(source["plans"], GatePlan);
	        this.staff = source["staff"];
	        this.role = source["role"];
	        this.daysThisWeek = source["daysThisWeek"];
	        this.entriesToday = source["entriesToday"];
	        this.allowed = source["allowed"];
	        this.reason = source["reason"];
	        this.lastAccess = this.convertValues(source["lastAccess"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GateMemberInput {
	    customerId: number;
	    blocked: boolean;
	    freeUntil: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new GateMemberInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customerId = source["customerId"];
	        this.blocked = source["blocked"];
	        this.freeUntil = source["freeUntil"];
	        this.notes = source["notes"];
	    }
	}
	
	export class GateStaffInput {
	    customerId: number;
	    name: string;
	    phone: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new GateStaffInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.customerId = source["customerId"];
	        this.name = source["name"];
	        this.phone = source["phone"];
	        this.role = source["role"];
	    }
	}
	export class GateStatus {
	    config: GateConfig;
	    serverRunning: boolean;
	    serverError: string;
	    // Go type: time
	    deviceLastSeen?: any;
	    localIps: string[];
	    serialPorts: string[];
	    allowedToday: number;
	    deniedToday: number;
	
	    static createFrom(source: any = {}) {
	        return new GateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], GateConfig);
	        this.serverRunning = source["serverRunning"];
	        this.serverError = source["serverError"];
	        this.deviceLastSeen = this.convertValues(source["deviceLastSeen"], null);
	        this.localIps = source["localIps"];
	        this.serialPorts = source["serialPorts"];
	        this.allowedToday = source["allowedToday"];
	        this.deniedToday = source["deniedToday"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GridInput {
	    modelId: number;
	    name: string;
	    categoryId?: number;
	    basePriceCents: number;
	    sizes: string[];
	    colors: string[];
	
	    static createFrom(source: any = {}) {
	        return new GridInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modelId = source["modelId"];
	        this.name = source["name"];
	        this.categoryId = source["categoryId"];
	        this.basePriceCents = source["basePriceCents"];
	        this.sizes = source["sizes"];
	        this.colors = source["colors"];
	    }
	}
	export class IdleProduct {
	    productId: number;
	    name: string;
	    categoryName: string;
	    stockQty: number;
	    costCents: number;
	    priceCents: number;
	    // Go type: time
	    lastSaleAt?: any;
	    idleDays: number;
	
	    static createFrom(source: any = {}) {
	        return new IdleProduct(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.name = source["name"];
	        this.categoryName = source["categoryName"];
	        this.stockQty = source["stockQty"];
	        this.costCents = source["costCents"];
	        this.priceCents = source["priceCents"];
	        this.lastSaleAt = this.convertValues(source["lastSaleAt"], null);
	        this.idleDays = source["idleDays"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportField {
	    key: string;
	    label: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.required = source["required"];
	    }
	}
	export class ImportFile {
	    path: string;
	    name: string;
	    headers: string[];
	    sample: string[][];
	    rows: number;
	    fields: ImportField[];
	    mapping: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new ImportFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.headers = source["headers"];
	        this.sample = source["sample"];
	        this.rows = source["rows"];
	        this.fields = this.convertValues(source["fields"], ImportField);
	        this.mapping = source["mapping"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportRow {
	    line: number;
	    name: string;
	    action: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.name = source["name"];
	        this.action = source["action"];
	        this.message = source["message"];
	    }
	}
	export class ImportPreview {
	    rows: ImportRow[];
	    new: number;
	    updated: number;
	    skipped: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], ImportRow);
	        this.new = source["new"];
	        this.updated = source["updated"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportRequest {
	    kind: string;
	    path: string;
	    mapping: Record<string, number>;
	    update: boolean;
	    supplierId: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.mapping = source["mapping"];
	        this.update = source["update"];
	        this.supplierId = source["supplierId"];
	    }
	}
	
	export class LicenseStatus {
	    state: string;
	    canWrite: boolean;
	    machineId: string;
	    store: string;
	    document: string;
	    plan: string;
	    expires: string;
	    daysLeft: number;
	    message: string;
	    warning: string;
	    licenseError: string;
	
	    static createFrom(source: any = {}) {
	        return new LicenseStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.canWrite = source["canWrite"];
	        this.machineId = source["machineId"];
	        this.store = source["store"];
	        this.document = source["document"];
	        this.plan = source["plan"];
	        this.expires = source["expires"];
	        this.daysLeft = source["daysLeft"];
	        this.message = source["message"];
	        this.warning = source["warning"];
	        this.licenseError = source["licenseError"];
	    }
	}
	export class NFeDecision {
	    number: number;
	    productId: number;
	    factor: number;
	    newPriceCents?: number;
	    name: string;
	    code: string;
	    skip: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NFeDecision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.productId = source["productId"];
	        this.factor = source["factor"];
	        this.newPriceCents = source["newPriceCents"];
	        this.name = source["name"];
	        this.code = source["code"];
	        this.skip = source["skip"];
	    }
	}
	export class NFeItem {
	    number: number;
	    code: string;
	    ean: string;
	    name: string;
	    ncm: string;
	    unit: string;
	    qtyText: string;
	    lineCents: number;
	    costCents: number;
	    productId?: number;
	    productName?: string;
	    productCode?: string;
	    matchedBy: string;
	    factor: number;
	    units: number;
	    unitCostCents: number;
	    priceCents: number;
	    currentCostCents: number;
	    problem: string;
	
	    static createFrom(source: any = {}) {
	        return new NFeItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.code = source["code"];
	        this.ean = source["ean"];
	        this.name = source["name"];
	        this.ncm = source["ncm"];
	        this.unit = source["unit"];
	        this.qtyText = source["qtyText"];
	        this.lineCents = source["lineCents"];
	        this.costCents = source["costCents"];
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.productCode = source["productCode"];
	        this.matchedBy = source["matchedBy"];
	        this.factor = source["factor"];
	        this.units = source["units"];
	        this.unitCostCents = source["unitCostCents"];
	        this.priceCents = source["priceCents"];
	        this.currentCostCents = source["currentCostCents"];
	        this.problem = source["problem"];
	    }
	}
	export class Supplier {
	    id: number;
	    legalName: string;
	    tradeName: string;
	    document: string;
	    stateRegistration: string;
	    contact: string;
	    phone: string;
	    email: string;
	    zipCode: string;
	    street: string;
	    number: string;
	    complement: string;
	    district: string;
	    city: string;
	    state: string;
	    notes: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Supplier(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.legalName = source["legalName"];
	        this.tradeName = source["tradeName"];
	        this.document = source["document"];
	        this.stateRegistration = source["stateRegistration"];
	        this.contact = source["contact"];
	        this.phone = source["phone"];
	        this.email = source["email"];
	        this.zipCode = source["zipCode"];
	        this.street = source["street"];
	        this.number = source["number"];
	        this.complement = source["complement"];
	        this.district = source["district"];
	        this.city = source["city"];
	        this.state = source["state"];
	        this.notes = source["notes"];
	        this.active = source["active"];
	    }
	}
	export class NFePreview {
	    accessKey: string;
	    number: string;
	    series: string;
	    issueDate: string;
	    supplier: Supplier;
	    alreadyImported: boolean;
	    invoiceCents: number;
	    items: NFeItem[];
	
	    static createFrom(source: any = {}) {
	        return new NFePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accessKey = source["accessKey"];
	        this.number = source["number"];
	        this.series = source["series"];
	        this.issueDate = source["issueDate"];
	        this.supplier = this.convertValues(source["supplier"], Supplier);
	        this.alreadyImported = source["alreadyImported"];
	        this.invoiceCents = source["invoiceCents"];
	        this.items = this.convertValues(source["items"], NFeItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OperatorTotal {
	    userName: string;
	    count: number;
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new OperatorTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userName = source["userName"];
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	    }
	}
	export class PayBillInput {
	    billId: number;
	    paidDate: string;
	    paidCents: number;
	    method: string;
	    fromCash: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PayBillInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.billId = source["billId"];
	        this.paidDate = source["paidDate"];
	        this.paidCents = source["paidCents"];
	        this.method = source["method"];
	        this.fromCash = source["fromCash"];
	    }
	}
	export class PayCreditInput {
	    saleIds: number[];
	    customerId: number;
	    amountCents: number;
	    paymentMethod: string;
	    notes: string;
	    requestId: string;
	
	    static createFrom(source: any = {}) {
	        return new PayCreditInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleIds = source["saleIds"];
	        this.customerId = source["customerId"];
	        this.amountCents = source["amountCents"];
	        this.paymentMethod = source["paymentMethod"];
	        this.notes = source["notes"];
	        this.requestId = source["requestId"];
	    }
	}
	export class PayInvoiceInput {
	    invoiceId: number;
	    paymentMethod: string;
	    payments: PaymentInput[];
	    requestId: string;
	    nextDue: string;
	
	    static createFrom(source: any = {}) {
	        return new PayInvoiceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.invoiceId = source["invoiceId"];
	        this.paymentMethod = source["paymentMethod"];
	        this.payments = this.convertValues(source["payments"], PaymentInput);
	        this.requestId = source["requestId"];
	        this.nextDue = source["nextDue"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class PermissionItem {
	    key: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new PermissionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	    }
	}
	export class PermissionCatalog {
	    items: PermissionItem[];
	    defaults: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new PermissionCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], PermissionItem);
	        this.defaults = source["defaults"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PixCode {
	    payload: string;
	    image: string;
	
	    static createFrom(source: any = {}) {
	        return new PixCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.payload = source["payload"];
	        this.image = source["image"];
	    }
	}
	export class ProductPromo {
	    label: string;
	    priceCents: number;
	    buyQty: number;
	    payQty: number;
	
	    static createFrom(source: any = {}) {
	        return new ProductPromo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.priceCents = source["priceCents"];
	        this.buyQty = source["buyQty"];
	        this.payQty = source["payQty"];
	    }
	}
	export class Product {
	    id: number;
	    name: string;
	    barcode?: string;
	    sku?: string;
	    priceCents: number;
	    stockQty: number;
	    active: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    categoryId?: number;
	    categoryName?: string;
	    modelId?: number;
	    modelName?: string;
	    size: string;
	    color: string;
	    costCents: number;
	    minStock: number;
	    pricePending: boolean;
	    noStock: boolean;
	    planDays?: number;
	    description?: string;
	    promo?: ProductPromo;
	
	    static createFrom(source: any = {}) {
	        return new Product(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.barcode = source["barcode"];
	        this.sku = source["sku"];
	        this.priceCents = source["priceCents"];
	        this.stockQty = source["stockQty"];
	        this.active = source["active"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.categoryId = source["categoryId"];
	        this.categoryName = source["categoryName"];
	        this.modelId = source["modelId"];
	        this.modelName = source["modelName"];
	        this.size = source["size"];
	        this.color = source["color"];
	        this.costCents = source["costCents"];
	        this.minStock = source["minStock"];
	        this.pricePending = source["pricePending"];
	        this.noStock = source["noStock"];
	        this.planDays = source["planDays"];
	        this.description = source["description"];
	        this.promo = this.convertValues(source["promo"], ProductPromo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProductCode {
	    sku: string;
	    barcode: string;
	
	    static createFrom(source: any = {}) {
	        return new ProductCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sku = source["sku"];
	        this.barcode = source["barcode"];
	    }
	}
	export class ProductInput {
	    id: number;
	    name: string;
	    barcode?: string;
	    sku?: string;
	    priceCents: number;
	    active: boolean;
	    initialStock: number;
	    categoryId?: number;
	    costCents?: number;
	    minStock: number;
	    autoSku: boolean;
	    autoBarcode: boolean;
	    noStock: boolean;
	    planDays?: number;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProductInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.barcode = source["barcode"];
	        this.sku = source["sku"];
	        this.priceCents = source["priceCents"];
	        this.active = source["active"];
	        this.initialStock = source["initialStock"];
	        this.categoryId = source["categoryId"];
	        this.costCents = source["costCents"];
	        this.minStock = source["minStock"];
	        this.autoSku = source["autoSku"];
	        this.autoBarcode = source["autoBarcode"];
	        this.noStock = source["noStock"];
	        this.planDays = source["planDays"];
	        this.description = source["description"];
	    }
	}
	export class ProductModel {
	    id: number;
	    name: string;
	    categoryId?: number;
	    basePriceCents: number;
	    sizes: string[];
	    colors: string[];
	    variationCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ProductModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.categoryId = source["categoryId"];
	        this.basePriceCents = source["basePriceCents"];
	        this.sizes = source["sizes"];
	        this.colors = source["colors"];
	        this.variationCount = source["variationCount"];
	    }
	}
	
	
	export class ProfitRow {
	    productId: number;
	    name: string;
	    categoryName: string;
	    qty: number;
	    revenueCents: number;
	    costCents: number;
	    profitCents: number;
	    estimated: boolean;
	    noCost: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfitRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.name = source["name"];
	        this.categoryName = source["categoryName"];
	        this.qty = source["qty"];
	        this.revenueCents = source["revenueCents"];
	        this.costCents = source["costCents"];
	        this.profitCents = source["profitCents"];
	        this.estimated = source["estimated"];
	        this.noCost = source["noCost"];
	    }
	}
	export class ProfitReport {
	    products: ProfitRow[];
	    categories: ProfitRow[];
	    revenueCents: number;
	    costCents: number;
	    profitCents: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfitReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.products = this.convertValues(source["products"], ProfitRow);
	        this.categories = this.convertValues(source["categories"], ProfitRow);
	        this.revenueCents = source["revenueCents"];
	        this.costCents = source["costCents"];
	        this.profitCents = source["profitCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Promotion {
	    id: number;
	    name: string;
	    kind: string;
	    productId?: number;
	    productName?: string;
	    categoryId?: number;
	    categoryName?: string;
	    priceCents: number;
	    percent: number;
	    buyQty: number;
	    payQty: number;
	    startDate: string;
	    endDate: string;
	    active: boolean;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Promotion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.categoryId = source["categoryId"];
	        this.categoryName = source["categoryName"];
	        this.priceCents = source["priceCents"];
	        this.percent = source["percent"];
	        this.buyQty = source["buyQty"];
	        this.payQty = source["payQty"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.active = source["active"];
	        this.status = source["status"];
	    }
	}
	export class Purchase {
	    id: number;
	    supplierId: number;
	    supplierName: string;
	    invoiceNumber: string;
	    issueDate: string;
	    accessKey?: string;
	    productsCents: number;
	    freightCents: number;
	    taxesCents: number;
	    discountCents: number;
	    totalCents: number;
	    notes: string;
	    userName?: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    cancelledAt?: any;
	    cancelledByName?: string;
	    cancellationReason?: string;
	    cancelledAccessKey?: string;
	
	    static createFrom(source: any = {}) {
	        return new Purchase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.supplierId = source["supplierId"];
	        this.supplierName = source["supplierName"];
	        this.invoiceNumber = source["invoiceNumber"];
	        this.issueDate = source["issueDate"];
	        this.accessKey = source["accessKey"];
	        this.productsCents = source["productsCents"];
	        this.freightCents = source["freightCents"];
	        this.taxesCents = source["taxesCents"];
	        this.discountCents = source["discountCents"];
	        this.totalCents = source["totalCents"];
	        this.notes = source["notes"];
	        this.userName = source["userName"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.cancelledAt = this.convertValues(source["cancelledAt"], null);
	        this.cancelledByName = source["cancelledByName"];
	        this.cancellationReason = source["cancellationReason"];
	        this.cancelledAccessKey = source["cancelledAccessKey"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PurchaseItem {
	    id: number;
	    productId: number;
	    productName: string;
	    qty: number;
	    unitCostCents: number;
	    totalCents: number;
	    costBeforeCents: number;
	    costAfterCents: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.qty = source["qty"];
	        this.unitCostCents = source["unitCostCents"];
	        this.totalCents = source["totalCents"];
	        this.costBeforeCents = source["costBeforeCents"];
	        this.costAfterCents = source["costAfterCents"];
	    }
	}
	export class PurchaseDetails {
	    purchase: Purchase;
	    items: PurchaseItem[];
	
	    static createFrom(source: any = {}) {
	        return new PurchaseDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.purchase = this.convertValues(source["purchase"], Purchase);
	        this.items = this.convertValues(source["items"], PurchaseItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PurchaseItemInput {
	    productId: number;
	    qty: number;
	    unitCostCents: number;
	    newPriceCents?: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.qty = source["qty"];
	        this.unitCostCents = source["unitCostCents"];
	        this.newPriceCents = source["newPriceCents"];
	    }
	}
	export class PurchaseInput {
	    supplierId: number;
	    invoiceNumber: string;
	    issueDate: string;
	    notes: string;
	    items: PurchaseItemInput[];
	    accessKey: string;
	    freightCents: number;
	    taxesCents: number;
	    discountCents: number;
	
	    static createFrom(source: any = {}) {
	        return new PurchaseInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supplierId = source["supplierId"];
	        this.invoiceNumber = source["invoiceNumber"];
	        this.issueDate = source["issueDate"];
	        this.notes = source["notes"];
	        this.items = this.convertValues(source["items"], PurchaseItemInput);
	        this.accessKey = source["accessKey"];
	        this.freightCents = source["freightCents"];
	        this.taxesCents = source["taxesCents"];
	        this.discountCents = source["discountCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Quote {
	    id: number;
	    customerId?: number;
	    customerName?: string;
	    status: string;
	    validUntil: string;
	    notes: string;
	    subtotalCents: number;
	    discountCents: number;
	    totalCents: number;
	    saleId?: number;
	    userName?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Quote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.status = source["status"];
	        this.validUntil = source["validUntil"];
	        this.notes = source["notes"];
	        this.subtotalCents = source["subtotalCents"];
	        this.discountCents = source["discountCents"];
	        this.totalCents = source["totalCents"];
	        this.saleId = source["saleId"];
	        this.userName = source["userName"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuoteDifference {
	    productName: string;
	    message: string;
	    blocking: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QuoteDifference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productName = source["productName"];
	        this.message = source["message"];
	        this.blocking = source["blocking"];
	    }
	}
	export class QuoteCheck {
	    differences: QuoteDifference[];
	    totalCents: number;
	
	    static createFrom(source: any = {}) {
	        return new QuoteCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.differences = this.convertValues(source["differences"], QuoteDifference);
	        this.totalCents = source["totalCents"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuoteConvertInput {
	    quoteId: number;
	    paymentMethod: string;
	    payments: PaymentInput[];
	    customerId?: number;
	    requestId: string;
	
	    static createFrom(source: any = {}) {
	        return new QuoteConvertInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quoteId = source["quoteId"];
	        this.paymentMethod = source["paymentMethod"];
	        this.payments = this.convertValues(source["payments"], PaymentInput);
	        this.customerId = source["customerId"];
	        this.requestId = source["requestId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuoteItem {
	    id: number;
	    productId: number;
	    productName: string;
	    barcode: string;
	    qty: number;
	    unitPriceCents: number;
	    discountCents: number;
	    promoDiscountCents: number;
	    promoLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new QuoteItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.barcode = source["barcode"];
	        this.qty = source["qty"];
	        this.unitPriceCents = source["unitPriceCents"];
	        this.discountCents = source["discountCents"];
	        this.promoDiscountCents = source["promoDiscountCents"];
	        this.promoLabel = source["promoLabel"];
	    }
	}
	export class QuoteDetails {
	    quote: Quote;
	    items: QuoteItem[];
	    customer?: Customer;
	
	    static createFrom(source: any = {}) {
	        return new QuoteDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quote = this.convertValues(source["quote"], Quote);
	        this.items = this.convertValues(source["items"], QuoteItem);
	        this.customer = this.convertValues(source["customer"], Customer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class QuoteInput {
	    id: number;
	    customerId?: number;
	    items: SaleItemInput[];
	    discountCents: number;
	    notes: string;
	    validUntil: string;
	
	    static createFrom(source: any = {}) {
	        return new QuoteInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.customerId = source["customerId"];
	        this.items = this.convertValues(source["items"], SaleItemInput);
	        this.discountCents = source["discountCents"];
	        this.notes = source["notes"];
	        this.validUntil = source["validUntil"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ReceiptLine {
	    kind: string;
	    text: string;
	    right: string;
	    bold: boolean;
	    center: boolean;
	    qty: string;
	    price: string;
	
	    static createFrom(source: any = {}) {
	        return new ReceiptLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.right = source["right"];
	        this.bold = source["bold"];
	        this.center = source["center"];
	        this.qty = source["qty"];
	        this.price = source["price"];
	    }
	}
	export class Receipt {
	    title: string;
	    logo: string;
	    lines: ReceiptLine[];
	
	    static createFrom(source: any = {}) {
	        return new Receipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.logo = source["logo"];
	        this.lines = this.convertValues(source["lines"], ReceiptLine);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ReturnItem {
	    saleItemId: number;
	    productName: string;
	    qty: number;
	    amountCents: number;
	
	    static createFrom(source: any = {}) {
	        return new ReturnItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleItemId = source["saleItemId"];
	        this.productName = source["productName"];
	        this.qty = source["qty"];
	        this.amountCents = source["amountCents"];
	    }
	}
	export class Return {
	    id: number;
	    saleId: number;
	    customerId?: number;
	    customerName?: string;
	    refundMethod: string;
	    totalCents: number;
	    reason: string;
	    userName?: string;
	    // Go type: time
	    createdAt: any;
	    items: ReturnItem[];
	
	    static createFrom(source: any = {}) {
	        return new Return(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.saleId = source["saleId"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.refundMethod = source["refundMethod"];
	        this.totalCents = source["totalCents"];
	        this.reason = source["reason"];
	        this.userName = source["userName"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.items = this.convertValues(source["items"], ReturnItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReturnableItem {
	    saleItemId: number;
	    productName: string;
	    qty: number;
	    returnedQty: number;
	    paidCents: number;
	
	    static createFrom(source: any = {}) {
	        return new ReturnableItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleItemId = source["saleItemId"];
	        this.productName = source["productName"];
	        this.qty = source["qty"];
	        this.returnedQty = source["returnedQty"];
	        this.paidCents = source["paidCents"];
	    }
	}
	export class ReturnInfo {
	    sale: Sale;
	    items: ReturnableItem[];
	    customer?: Customer;
	    hasFiado: boolean;
	    cardLeftCents: number;
	    methods: string[];
	    returns: Return[];
	
	    static createFrom(source: any = {}) {
	        return new ReturnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sale = this.convertValues(source["sale"], Sale);
	        this.items = this.convertValues(source["items"], ReturnableItem);
	        this.customer = this.convertValues(source["customer"], Customer);
	        this.hasFiado = source["hasFiado"];
	        this.cardLeftCents = source["cardLeftCents"];
	        this.methods = source["methods"];
	        this.returns = this.convertValues(source["returns"], Return);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReturnItemInput {
	    saleItemId: number;
	    qty: number;
	
	    static createFrom(source: any = {}) {
	        return new ReturnItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleItemId = source["saleItemId"];
	        this.qty = source["qty"];
	    }
	}
	export class ReturnInput {
	    saleId: number;
	    items: ReturnItemInput[];
	    refundMethod: string;
	    customerId?: number;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ReturnInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saleId = source["saleId"];
	        this.items = this.convertValues(source["items"], ReturnItemInput);
	        this.refundMethod = source["refundMethod"];
	        this.customerId = source["customerId"];
	        this.reason = source["reason"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class SalePayment {
	    id: number;
	    method: string;
	    amountCents: number;
	    receivedCents: number;
	    changeCents: number;
	    cardBrand: string;
	    installments: number;
	
	    static createFrom(source: any = {}) {
	        return new SalePayment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.method = source["method"];
	        this.amountCents = source["amountCents"];
	        this.receivedCents = source["receivedCents"];
	        this.changeCents = source["changeCents"];
	        this.cardBrand = source["cardBrand"];
	        this.installments = source["installments"];
	    }
	}
	export class SaleItem {
	    id: number;
	    saleId: number;
	    productId: number;
	    productNameSnapshot: string;
	    qty: number;
	    unitPriceCents: number;
	    subtotalCents: number;
	    discountCents: number;
	    promotionId?: number;
	    promoDiscountCents: number;
	    promoLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new SaleItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.saleId = source["saleId"];
	        this.productId = source["productId"];
	        this.productNameSnapshot = source["productNameSnapshot"];
	        this.qty = source["qty"];
	        this.unitPriceCents = source["unitPriceCents"];
	        this.subtotalCents = source["subtotalCents"];
	        this.discountCents = source["discountCents"];
	        this.promotionId = source["promotionId"];
	        this.promoDiscountCents = source["promoDiscountCents"];
	        this.promoLabel = source["promoLabel"];
	    }
	}
	export class SaleDetails {
	    sale: Sale;
	    items: SaleItem[];
	    payments: SalePayment[];
	    customer?: Customer;
	    delivery?: Delivery;
	    returns: Return[];
	
	    static createFrom(source: any = {}) {
	        return new SaleDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sale = this.convertValues(source["sale"], Sale);
	        this.items = this.convertValues(source["items"], SaleItem);
	        this.payments = this.convertValues(source["payments"], SalePayment);
	        this.customer = this.convertValues(source["customer"], Customer);
	        this.delivery = this.convertValues(source["delivery"], Delivery);
	        this.returns = this.convertValues(source["returns"], Return);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class StockMovement {
	    id: number;
	    productId: number;
	    productName: string;
	    qtyChange: number;
	    reason: string;
	    referenceType?: string;
	    referenceId?: number;
	    notes?: string;
	    userName?: string;
	    unitCents?: number;
	    isCost: boolean;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new StockMovement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.qtyChange = source["qtyChange"];
	        this.reason = source["reason"];
	        this.referenceType = source["referenceType"];
	        this.referenceId = source["referenceId"];
	        this.notes = source["notes"];
	        this.userName = source["userName"];
	        this.unitCents = source["unitCents"];
	        this.isCost = source["isCost"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SalesReport {
	    id?: number;
	    startDate: string;
	    endDate: string;
	    saleCount: number;
	    totalCents: number;
	    discountCents: number;
	    cancelledCount: number;
	    cancelledCents: number;
	    byPaymentMethod: PaymentTotal[];
	    products: ProductSold[];
	    notSold: ProductSold[];
	    lowStockDefault: number;
	    sales: Sale[];
	    creditPayments: CreditPayment[];
	    creditReceivedCents: number;
	    stockMovements: StockMovement[];
	    byOperator: OperatorTotal[];
	    byCategory: CategoryTotal[];
	
	    static createFrom(source: any = {}) {
	        return new SalesReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.saleCount = source["saleCount"];
	        this.totalCents = source["totalCents"];
	        this.discountCents = source["discountCents"];
	        this.cancelledCount = source["cancelledCount"];
	        this.cancelledCents = source["cancelledCents"];
	        this.byPaymentMethod = this.convertValues(source["byPaymentMethod"], PaymentTotal);
	        this.products = this.convertValues(source["products"], ProductSold);
	        this.notSold = this.convertValues(source["notSold"], ProductSold);
	        this.lowStockDefault = source["lowStockDefault"];
	        this.sales = this.convertValues(source["sales"], Sale);
	        this.creditPayments = this.convertValues(source["creditPayments"], CreditPayment);
	        this.creditReceivedCents = source["creditReceivedCents"];
	        this.stockMovements = this.convertValues(source["stockMovements"], StockMovement);
	        this.byOperator = this.convertValues(source["byOperator"], OperatorTotal);
	        this.byCategory = this.convertValues(source["byCategory"], CategoryTotal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SavedReport {
	    id: number;
	    kind: string;
	    startDate: string;
	    endDate: string;
	    filterUserName?: string;
	    saleCount: number;
	    totalCents: number;
	    userName?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new SavedReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.filterUserName = source["filterUserName"];
	        this.saleCount = source["saleCount"];
	        this.totalCents = source["totalCents"];
	        this.userName = source["userName"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class User {
	    id: number;
	    name: string;
	    login: string;
	    role: string;
	    active: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    permissions: string[];
	    loggedIn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.login = source["login"];
	        this.role = source["role"];
	        this.active = source["active"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.permissions = source["permissions"];
	        this.loggedIn = source["loggedIn"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Session {
	    user?: User;
	    needsSetup: boolean;
	    permissions: string[];
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], User);
	        this.needsSetup = source["needsSetup"];
	        this.permissions = source["permissions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    storeName: string;
	    autoReceipt: boolean;
	    legalName: string;
	    document: string;
	    stateRegistration: string;
	    phone: string;
	    whatsapp: string;
	    email: string;
	    zipCode: string;
	    street: string;
	    number: string;
	    complement: string;
	    district: string;
	    city: string;
	    state: string;
	    logo: string;
	    receiptFooter: string;
	    quoteValidityDays: number;
	    lowStockDefault: number;
	    paperWidth: string;
	    printerName: string;
	    cutPaper: boolean;
	    openDrawer: boolean;
	    lockMinutes: number;
	    operatorMaxDiscountPct: number;
	    nfeCostFreight: boolean;
	    nfeCostIpi: boolean;
	    nfeCostSt: boolean;
	    nfeCostOther: boolean;
	    nfeCostDiscount: boolean;
	    pixKey: string;
	    pixName: string;
	    pixCity: string;
	    chargeMessage: string;
	    setupDone: boolean;
	    cashPerUser: boolean;
	    msgBirthday: string;
	    msgDue: string;
	    msgOverdue: string;
	    msgMissing: string;
	    msgWinback: string;
	    dueDays: number;
	    missingDays: number;
	    backupAuto: boolean;
	    backupKeep: number;
	    backupDir: string;
	    backupExternalDir: string;
	    backupWarnDays: number;
	    pixOnReceipt: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.storeName = source["storeName"];
	        this.autoReceipt = source["autoReceipt"];
	        this.legalName = source["legalName"];
	        this.document = source["document"];
	        this.stateRegistration = source["stateRegistration"];
	        this.phone = source["phone"];
	        this.whatsapp = source["whatsapp"];
	        this.email = source["email"];
	        this.zipCode = source["zipCode"];
	        this.street = source["street"];
	        this.number = source["number"];
	        this.complement = source["complement"];
	        this.district = source["district"];
	        this.city = source["city"];
	        this.state = source["state"];
	        this.logo = source["logo"];
	        this.receiptFooter = source["receiptFooter"];
	        this.quoteValidityDays = source["quoteValidityDays"];
	        this.lowStockDefault = source["lowStockDefault"];
	        this.paperWidth = source["paperWidth"];
	        this.printerName = source["printerName"];
	        this.cutPaper = source["cutPaper"];
	        this.openDrawer = source["openDrawer"];
	        this.lockMinutes = source["lockMinutes"];
	        this.operatorMaxDiscountPct = source["operatorMaxDiscountPct"];
	        this.nfeCostFreight = source["nfeCostFreight"];
	        this.nfeCostIpi = source["nfeCostIpi"];
	        this.nfeCostSt = source["nfeCostSt"];
	        this.nfeCostOther = source["nfeCostOther"];
	        this.nfeCostDiscount = source["nfeCostDiscount"];
	        this.pixKey = source["pixKey"];
	        this.pixName = source["pixName"];
	        this.pixCity = source["pixCity"];
	        this.chargeMessage = source["chargeMessage"];
	        this.setupDone = source["setupDone"];
	        this.cashPerUser = source["cashPerUser"];
	        this.msgBirthday = source["msgBirthday"];
	        this.msgDue = source["msgDue"];
	        this.msgOverdue = source["msgOverdue"];
	        this.msgMissing = source["msgMissing"];
	        this.msgWinback = source["msgWinback"];
	        this.dueDays = source["dueDays"];
	        this.missingDays = source["missingDays"];
	        this.backupAuto = source["backupAuto"];
	        this.backupKeep = source["backupKeep"];
	        this.backupDir = source["backupDir"];
	        this.backupExternalDir = source["backupExternalDir"];
	        this.backupWarnDays = source["backupWarnDays"];
	        this.pixOnReceipt = source["pixOnReceipt"];
	    }
	}
	
	export class StockAuditEntry {
	    productId: number;
	    productName: string;
	    storedQty: number;
	    calculatedQty: number;
	    differenceQty: number;
	
	    static createFrom(source: any = {}) {
	        return new StockAuditEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.productId = source["productId"];
	        this.productName = source["productName"];
	        this.storedQty = source["storedQty"];
	        this.calculatedQty = source["calculatedQty"];
	        this.differenceQty = source["differenceQty"];
	    }
	}
	
	export class StockValueRow {
	    categoryName: string;
	    products: number;
	    qty: number;
	    costCents: number;
	    priceCents: number;
	    noCost: number;
	
	    static createFrom(source: any = {}) {
	        return new StockValueRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categoryName = source["categoryName"];
	        this.products = source["products"];
	        this.qty = source["qty"];
	        this.costCents = source["costCents"];
	        this.priceCents = source["priceCents"];
	        this.noCost = source["noCost"];
	    }
	}
	export class SubscriptionInvoice {
	    id: number;
	    dueDate: string;
	    amountCents: number;
	    status: string;
	    imported: boolean;
	    outsideCash: boolean;
	    saleId?: number;
	    // Go type: time
	    paidAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionInvoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.dueDate = source["dueDate"];
	        this.amountCents = source["amountCents"];
	        this.status = source["status"];
	        this.imported = source["imported"];
	        this.outsideCash = source["outsideCash"];
	        this.saleId = source["saleId"];
	        this.paidAt = this.convertValues(source["paidAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Subscription {
	    id: number;
	    customerId: number;
	    customerName: string;
	    customerPhone?: string;
	    productId: number;
	    planName: string;
	    planDays: number;
	    priceCents?: number;
	    rules?: AccessRules;
	    status: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    cancelledAt?: any;
	    // Go type: time
	    pausedAt?: any;
	    pauseReason?: string;
	    openInvoice?: SubscriptionInvoice;
	    overdue: boolean;
	    // Go type: time
	    lastPaidAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Subscription(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.customerPhone = source["customerPhone"];
	        this.productId = source["productId"];
	        this.planName = source["planName"];
	        this.planDays = source["planDays"];
	        this.priceCents = source["priceCents"];
	        this.rules = this.convertValues(source["rules"], AccessRules);
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.cancelledAt = this.convertValues(source["cancelledAt"], null);
	        this.pausedAt = this.convertValues(source["pausedAt"], null);
	        this.pauseReason = source["pauseReason"];
	        this.openInvoice = this.convertValues(source["openInvoice"], SubscriptionInvoice);
	        this.overdue = source["overdue"];
	        this.lastPaidAt = this.convertValues(source["lastPaidAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SubscriptionImportPlan {
	    name: string;
	    priceCents: number;
	    days: number;
	    invoices: number;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionImportPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.priceCents = source["priceCents"];
	        this.days = source["days"];
	        this.invoices = source["invoices"];
	        this.exists = source["exists"];
	    }
	}
	export class SubscriptionImport {
	    applied: boolean;
	    customersNew: number;
	    customersExisting: number;
	    customersWithSub: number;
	    customersSkipped: number;
	    skipped: string[];
	    plans: SubscriptionImportPlan[];
	    subsActive: number;
	    subsCancelled: number;
	    invoicesPaid: number;
	    invoicesOpen: number;
	    invoicesUnpaidOld: number;
	    invoicesSkipped: number;
	    otherLines: number;
	    fixedWords: number;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionImport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applied = source["applied"];
	        this.customersNew = source["customersNew"];
	        this.customersExisting = source["customersExisting"];
	        this.customersWithSub = source["customersWithSub"];
	        this.customersSkipped = source["customersSkipped"];
	        this.skipped = source["skipped"];
	        this.plans = this.convertValues(source["plans"], SubscriptionImportPlan);
	        this.subsActive = source["subsActive"];
	        this.subsCancelled = source["subsCancelled"];
	        this.invoicesPaid = source["invoicesPaid"];
	        this.invoicesOpen = source["invoicesOpen"];
	        this.invoicesUnpaidOld = source["invoicesUnpaidOld"];
	        this.invoicesSkipped = source["invoicesSkipped"];
	        this.otherLines = source["otherLines"];
	        this.fixedWords = source["fixedWords"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class SupplierPurchases {
	    supplierId: number;
	    name: string;
	    count: number;
	    totalCents: number;
	    // Go type: time
	    lastAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new SupplierPurchases(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supplierId = source["supplierId"];
	        this.name = source["name"];
	        this.count = source["count"];
	        this.totalCents = source["totalCents"];
	        this.lastAt = this.convertValues(source["lastAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestPurgeSession {
	    id: number;
	    // Go type: time
	    openedAt: any;
	    status: string;
	    movements: number;
	
	    static createFrom(source: any = {}) {
	        return new TestPurgeSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.openedAt = this.convertValues(source["openedAt"], null);
	        this.status = source["status"];
	        this.movements = source["movements"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestPurgeSale {
	    id: number;
	    // Go type: time
	    at: any;
	    totalCents: number;
	    paymentMethod: string;
	    status: string;
	    customerName: string;
	
	    static createFrom(source: any = {}) {
	        return new TestPurgeSale(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = this.convertValues(source["at"], null);
	        this.totalCents = source["totalCents"];
	        this.paymentMethod = source["paymentMethod"];
	        this.status = source["status"];
	        this.customerName = source["customerName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestPurge {
	    date: string;
	    userId: number;
	    sales: TestPurgeSale[];
	    sessions: TestPurgeSession[];
	    totalCents: number;
	    touchedSessions: number[];
	    blockers: string[];
	    backupPath: string;
	    applied: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TestPurge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.userId = source["userId"];
	        this.sales = this.convertValues(source["sales"], TestPurgeSale);
	        this.sessions = this.convertValues(source["sessions"], TestPurgeSession);
	        this.totalCents = source["totalCents"];
	        this.touchedSessions = source["touchedSessions"];
	        this.blockers = source["blockers"];
	        this.backupPath = source["backupPath"];
	        this.applied = source["applied"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class UserInput {
	    id: number;
	    name: string;
	    login: string;
	    password: string;
	    role: string;
	    active: boolean;
	    permissions: string[];
	
	    static createFrom(source: any = {}) {
	        return new UserInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.login = source["login"];
	        this.password = source["password"];
	        this.role = source["role"];
	        this.active = source["active"];
	        this.permissions = source["permissions"];
	    }
	}

}

