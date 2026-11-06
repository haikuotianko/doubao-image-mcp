export namespace doubao {
	
	export class Model {
	    name: string;
	    id: number;
	    sub: string;
	    needUpgrade: boolean;
	    default: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.id = source["id"];
	        this.sub = source["sub"];
	        this.needUpgrade = source["needUpgrade"];
	        this.default = source["default"];
	    }
	}
	export class Style {
	    key: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Style(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	    }
	}

}

export namespace main {
	
	export class AccountView {
	    name: string;
	    nickname: string;
	    avatar: string;
	    enabled: boolean;
	    busy: boolean;
	    active: boolean;
	    addedAt: string;
	    remark: string;
	    cookieCount: number;
	
	    static createFrom(source: any = {}) {
	        return new AccountView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.enabled = source["enabled"];
	        this.busy = source["busy"];
	        this.active = source["active"];
	        this.addedAt = source["addedAt"];
	        this.remark = source["remark"];
	        this.cookieCount = source["cookieCount"];
	    }
	}
	export class LoginResult {
	    account: string;
	    cookies: string[];
	    verified: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = source["account"];
	        this.cookies = source["cookies"];
	        this.verified = source["verified"];
	        this.message = source["message"];
	    }
	}
	export class Settings {
	    outDir: string;
	    defaultModel: string;
	    defaultRatio: string;
	    defaultStyle: string;
	    fitRatio: boolean;
	    httpAddr: string;
	    autoStartHttp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outDir = source["outDir"];
	        this.defaultModel = source["defaultModel"];
	        this.defaultRatio = source["defaultRatio"];
	        this.defaultStyle = source["defaultStyle"];
	        this.fitRatio = source["fitRatio"];
	        this.httpAddr = source["httpAddr"];
	        this.autoStartHttp = source["autoStartHttp"];
	    }
	}
	export class Status {
	    activeAccount: string;
	    accounts: AccountView[];
	    waiting: number;
	    httpRunning: boolean;
	    httpAddr: string;
	    outDir: string;
	    defaultModel: string;
	    defaultRatio: string;
	    defaultStyle: string;
	    fitRatio: boolean;
	    autoStartHttp: boolean;
	    configPath: string;
	    exePath: string;
	    platform: string;
	    loginSupported: boolean;
	    models: doubao.Model[];
	    styles: doubao.Style[];
	    ratios: string[];
	    logs: string[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.activeAccount = source["activeAccount"];
	        this.accounts = this.convertValues(source["accounts"], AccountView);
	        this.waiting = source["waiting"];
	        this.httpRunning = source["httpRunning"];
	        this.httpAddr = source["httpAddr"];
	        this.outDir = source["outDir"];
	        this.defaultModel = source["defaultModel"];
	        this.defaultRatio = source["defaultRatio"];
	        this.defaultStyle = source["defaultStyle"];
	        this.fitRatio = source["fitRatio"];
	        this.autoStartHttp = source["autoStartHttp"];
	        this.configPath = source["configPath"];
	        this.exePath = source["exePath"];
	        this.platform = source["platform"];
	        this.loginSupported = source["loginSupported"];
	        this.models = this.convertValues(source["models"], doubao.Model);
	        this.styles = this.convertValues(source["styles"], doubao.Style);
	        this.ratios = source["ratios"];
	        this.logs = source["logs"];
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
	export class TestResult {
	    ok: boolean;
	    message: string;
	    images: mcpserver.ImageItem[];
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.images = this.convertValues(source["images"], mcpserver.ImageItem);
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

}

export namespace mcpserver {
	
	export class ImageItem {
	    path?: string;
	    url?: string;
	    width?: number;
	    height?: number;
	    format?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.url = source["url"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.format = source["format"];
	    }
	}

}

