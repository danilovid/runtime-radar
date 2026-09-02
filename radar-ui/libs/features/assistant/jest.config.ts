export default {
    displayName: 'feature-assistant',
    preset: '../../../jest.preset.js',
    setupFilesAfterEnv: [
        '<rootDir>/src/test-setup.ts'
    ],
    globals: {
        'ts-jest': {
            stringifyContentPathRegex: '\\.(html|svg)$',
            tsconfig: '<rootDir>/tsconfig.spec.json'
        }
    },
    coverageDirectory: '../../../coverage/libs/feature-assistant',
    snapshotSerializers: [
        'jest-preset-angular/build/serializers/no-ng-attributes',
        'jest-preset-angular/build/serializers/ng-snapshot',
        'jest-preset-angular/build/serializers/html-comment'
    ],
    // Angular and zone.js ship .mjs, which jest cannot parse untransformed:
    // everything but .mjs under node_modules stays ignored.
    transformIgnorePatterns: [
        'node_modules/(?!.*\\.mjs$)'
    ],
    transform: {
        '^.+\\.(ts|mjs|js|html)$': 'jest-preset-angular'
    }
};
