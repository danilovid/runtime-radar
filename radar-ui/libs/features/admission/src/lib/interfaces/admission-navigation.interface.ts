export enum AdmissionRouterName {
    SETTINGS = 'settings'
}

export interface AdmissionNavigationTab {
    path: AdmissionRouterName;
    localizationKey: string;
}
