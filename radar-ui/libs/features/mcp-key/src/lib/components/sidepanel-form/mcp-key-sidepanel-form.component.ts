import { DateAdapter } from '@koobiq/components/core';
import { DateTime } from 'luxon';
import { KbqAlertColors } from '@koobiq/components/alert';
import { ChangeDetectionStrategy, Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, FormRecord, Validators } from '@angular/forms';
import { KBQ_SIDEPANEL_DATA, KbqSidepanelRef } from '@koobiq/components/sidepanel';
import { Observable, debounceTime, distinctUntilChanged, distinctUntilKeyChanged, map, startWith, tap } from 'rxjs';

import { FormScheme, CoreUtilsService as utils } from '@cs/core';
import { McpKeyPermissionName, McpKeyScope } from '@cs/domains/mcp-key';
import { PermissionName, PermissionType } from '@cs/domains/role';

import { McpKeySidepanelFormProps } from '../../interfaces/mcp-key-sidepanel.interface';
import {
    McpKeyExpiryDatePreset,
    McpKeyExpiryDatePresetOption,
    McpKeyForm,
    McpKeyPermissionRecord,
    McpKeyPermissionType,
    McpKeyScopeOption
} from '../../interfaces/mcp-key-form.interface';

const MCP_KEY_EXPIRY_DATE_PRESET: McpKeyExpiryDatePresetOption[] = [
    {
        id: McpKeyExpiryDatePreset.WEEK,
        localizationKey: 'McpKey.Pseudo.ExpiryDatePreset.Week'
    },
    {
        id: McpKeyExpiryDatePreset.MONTH,
        localizationKey: 'McpKey.Pseudo.ExpiryDatePreset.Month'
    },
    {
        id: McpKeyExpiryDatePreset.QUARTER,
        localizationKey: 'McpKey.Pseudo.ExpiryDatePreset.Quarter'
    },
    {
        id: McpKeyExpiryDatePreset.INDEFINITELY,
        localizationKey: 'McpKey.Pseudo.ExpiryDatePreset.Indefinitely'
    },
    {
        id: McpKeyExpiryDatePreset.CUSTOM,
        localizationKey: 'McpKey.Pseudo.ExpiryDatePreset.Custom'
    }
];

// MCP_KEY_SCOPES are the halves of the product a key can be limited to. They are
// a property of the key rather than of a role: both monitors are guarded by the
// same permission, and this is what tells an agent's key apart from a colleague's.
const MCP_KEY_SCOPES: McpKeyScopeOption[] = [
    {
        id: McpKeyScope.RUNTIME_MONITOR,
        localizationKey: 'McpKey.Pseudo.Scope.RuntimeMonitor',
        descriptionLocalizationKey: 'McpKey.Pseudo.Scope.RuntimeMonitorDescription'
    },
    {
        id: McpKeyScope.ADMISSION,
        localizationKey: 'McpKey.Pseudo.Scope.Admission',
        descriptionLocalizationKey: 'McpKey.Pseudo.Scope.AdmissionDescription'
    }
];

@Component({
    templateUrl: './mcp-key-sidepanel-form.component.html',
    styleUrl: './mcp-key-sidepanel-form.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class McpKeyFeatureSidepanelFormComponent implements OnInit {
    readonly form: FormGroup<FormScheme<McpKeyForm, 'permissions'>> = this.formBuilder.group({
        name: ['', Validators.required],
        date: [this.dateAdapter.today()],
        preset: [McpKeyExpiryDatePreset.WEEK],
        permissions: this.formBuilder.record<McpKeyPermissionRecord>({}),
        scopes: [[] as McpKeyScope[]]
    });

    readonly expiryDatePreset$: Observable<McpKeyExpiryDatePreset> = this.form.valueChanges.pipe(
        startWith(this.form.value),
        distinctUntilKeyChanged('preset'),
        map(() => utils.getFormValues<McpKeyForm>(this.form.controls).preset),
        tap((preset) => {
            const control = this.form.get('date');
            utils.toggleControlEnable(control, preset === McpKeyExpiryDatePreset.CUSTOM);

            switch (preset) {
                case McpKeyExpiryDatePreset.WEEK:
                    control?.setValue(this.dateAdapter.today().plus({ days: 7 }), { onlySelf: true });
                    break;
                case McpKeyExpiryDatePreset.MONTH:
                    control?.setValue(this.dateAdapter.today().plus({ months: 1 }), { onlySelf: true });
                    break;
                case McpKeyExpiryDatePreset.QUARTER:
                    control?.setValue(this.dateAdapter.today().plus({ months: 3 }), { onlySelf: true });
                    break;
                case McpKeyExpiryDatePreset.INDEFINITELY:
                    control?.setValue(null, { onlySelf: true });
                    break;
                case McpKeyExpiryDatePreset.CUSTOM:
                    control?.setValue(this.dateAdapter.today().plus({ days: 1 }), { onlySelf: true });
                    break;
            }
        })
    );

    readonly isFormValid$: Observable<boolean> = this.form.valueChanges.pipe(
        startWith(this.form.value),
        /* eslint @typescript-eslint/no-magic-numbers: "off" */
        debounceTime(250),
        distinctUntilChanged(),
        map(() => {
            const permissions = utils.getFormValues<McpKeyForm>(this.form.controls).permissions;
            const arePermissionsValid = Object.values(permissions)
                .reduce((acc, item) => acc.concat(Object.values(item)), [] as boolean[])
                .some((item) => !!item);

            return utils.isFormValid(this.form.controls) && arePermissionsValid;
        })
    );

    get permissionsFormGroup(): FormRecord {
        return this.form.get('permissions') as FormRecord;
    }

    readonly minDate = this.dateAdapter.today().plus({ days: 1 });

    readonly dateTimeFullFormat = DateTime.DATETIME_FULL;

    readonly mcpKeyExpiryDatePresetOptions: McpKeyExpiryDatePresetOption[] = MCP_KEY_EXPIRY_DATE_PRESET;

    readonly mcpKeyScopeOptions: McpKeyScopeOption[] = MCP_KEY_SCOPES;

    readonly mcpKeyExpiryDatePreset = McpKeyExpiryDatePreset;

    readonly mcpKeyPermissionType = McpKeyPermissionType;

    readonly permissionType = PermissionType;

    readonly permissionName = PermissionName;

    readonly alertColors = KbqAlertColors;

    constructor(
        private readonly dateAdapter: DateAdapter<DateTime>,
        private readonly formBuilder: FormBuilder,
        private readonly sidepanelRef: KbqSidepanelRef,
        @Inject(KBQ_SIDEPANEL_DATA) public readonly props: McpKeySidepanelFormProps
    ) {}

    ngOnInit() {
        const rulePermissionsForm = this.formBuilder.group({
            [McpKeyPermissionType.CREATE]: [
                { value: false, disabled: !this.props.permissions[PermissionName.RULES].has(PermissionType.CREATE) }
            ],
            [McpKeyPermissionType.READ]: [
                { value: false, disabled: !this.props.permissions[PermissionName.RULES].has(PermissionType.READ) }
            ],
            [McpKeyPermissionType.UPDATE]: [
                { value: false, disabled: !this.props.permissions[PermissionName.RULES].has(PermissionType.UPDATE) }
            ],
            [McpKeyPermissionType.DELETE]: [
                { value: false, disabled: !this.props.permissions[PermissionName.RULES].has(PermissionType.DELETE) }
            ]
        });

        const eventPermissionsForm = this.formBuilder.group({
            [McpKeyPermissionType.READ]: [
                { value: false, disabled: !this.props.permissions[PermissionName.EVENTS].has(PermissionType.READ) }
            ]
        });

        // System settings is what the admission source tools need: the product
        // guards both monitors' configuration with it.
        const systemSettingsPermissionsForm = this.formBuilder.group({
            [McpKeyPermissionType.READ]: [
                { value: false, disabled: !this.props.permissions[PermissionName.SYSTEM].has(PermissionType.READ) }
            ],
            [McpKeyPermissionType.UPDATE]: [
                { value: false, disabled: !this.props.permissions[PermissionName.SYSTEM].has(PermissionType.UPDATE) }
            ]
        });

        this.permissionsFormGroup.addControl(McpKeyPermissionName.RULES, rulePermissionsForm);
        this.permissionsFormGroup.addControl(McpKeyPermissionName.EVENTS, eventPermissionsForm);
        this.permissionsFormGroup.addControl(McpKeyPermissionName.SYSTEM_SETTINGS, systemSettingsPermissionsForm);
    }

    /** Whether the key is limited to the given half of the product. */
    isScopeSelected(scope: McpKeyScope): boolean {
        return (this.form.controls.scopes.value ?? []).includes(scope);
    }

    // The checkboxes carry a list rather than a boolean each, so the control is
    // maintained by hand: there is no form control for a single scope to bind.
    toggleScope(scope: McpKeyScope) {
        const control = this.form.controls.scopes;
        const selected = control.value ?? [];

        control.setValue(selected.includes(scope) ? selected.filter((item) => item !== scope) : [...selected, scope]);
    }

    confirm() {
        const formValues = utils.getFormValues<McpKeyForm>(this.form.controls);
        this.sidepanelRef.close(utils.getTrimmedFormValues<McpKeyForm>(formValues));
    }

    cancel() {
        this.sidepanelRef.close(undefined);
    }
}
