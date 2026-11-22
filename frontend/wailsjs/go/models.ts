export namespace main {
	
	export class DiagResult {
	    isAdmin: boolean;
	    adapterName: string;
	    localIP: string;
	    internetOK: boolean;
	    pingMS: number;
	    dnsOK: boolean;
	    proxyEnabled: boolean;
	    proxyServer: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isAdmin = source["isAdmin"];
	        this.adapterName = source["adapterName"];
	        this.localIP = source["localIP"];
	        this.internetOK = source["internetOK"];
	        this.pingMS = source["pingMS"];
	        this.dnsOK = source["dnsOK"];
	        this.proxyEnabled = source["proxyEnabled"];
	        this.proxyServer = source["proxyServer"];
	    }
	}
	export class FixOptions {
	    dnsMode: string;
	    resetIP: boolean;
	    advanced: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FixOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dnsMode = source["dnsMode"];
	        this.resetIP = source["resetIP"];
	        this.advanced = source["advanced"];
	    }
	}

}

