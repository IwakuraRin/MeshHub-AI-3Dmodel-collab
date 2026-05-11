export namespace main {
	
	export class AnalysisDetail {
	    label: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.value = source["value"];
	    }
	}
	export class ModelBounds {
	    minX: number;
	    minY: number;
	    minZ: number;
	    maxX: number;
	    maxY: number;
	    maxZ: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelBounds(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minX = source["minX"];
	        this.minY = source["minY"];
	        this.minZ = source["minZ"];
	        this.maxX = source["maxX"];
	        this.maxY = source["maxY"];
	        this.maxZ = source["maxZ"];
	    }
	}
	export class ModelAnalysis {
	    id: string;
	    path: string;
	    name: string;
	    extension: string;
	    size: number;
	    sizeLabel: string;
	    formatName: string;
	    formatFamily: string;
	    previewable: boolean;
	    needsConversion: boolean;
	    previewFormat: string;
	    summary: string;
	    details: AnalysisDetail[];
	    bounds?: ModelBounds;
	
	    static createFrom(source: any = {}) {
	        return new ModelAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.extension = source["extension"];
	        this.size = source["size"];
	        this.sizeLabel = source["sizeLabel"];
	        this.formatName = source["formatName"];
	        this.formatFamily = source["formatFamily"];
	        this.previewable = source["previewable"];
	        this.needsConversion = source["needsConversion"];
	        this.previewFormat = source["previewFormat"];
	        this.summary = source["summary"];
	        this.details = this.convertValues(source["details"], AnalysisDetail);
	        this.bounds = this.convertValues(source["bounds"], ModelBounds);
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
	
	export class PreviewModelPayload {
	    fileName: string;
	    extension: string;
	    mimeType: string;
	    base64: string;
	
	    static createFrom(source: any = {}) {
	        return new PreviewModelPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileName = source["fileName"];
	        this.extension = source["extension"];
	        this.mimeType = source["mimeType"];
	        this.base64 = source["base64"];
	    }
	}
	export class UploadModelPayload {
	    fileName: string;
	    extension: string;
	    size: number;
	    mimeType: string;
	    base64: string;
	
	    static createFrom(source: any = {}) {
	        return new UploadModelPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileName = source["fileName"];
	        this.extension = source["extension"];
	        this.size = source["size"];
	        this.mimeType = source["mimeType"];
	        this.base64 = source["base64"];
	    }
	}

}

