# [2.0.0](https://github.com/NoTIPswe/notip-simulator-backend/compare/v1.3.0...v2.0.0) (2026-04-11)


### Bug Fixes

* update README ([9dd37d9](https://github.com/NoTIPswe/notip-simulator-backend/commit/9dd37d9981af4363ff31295ec171cdf7d00355e5))
* update README ([51d7f6f](https://github.com/NoTIPswe/notip-simulator-backend/commit/51d7f6f9770750f41ac6a53edd8e8b5faf1b8ff7))


### BREAKING CHANGES

* new version
* new version

# [1.3.0](https://github.com/NoTIPswe/notip-simulator-backend/compare/v1.2.0...v1.3.0) (2026-04-10)


### Features

* bulk creation ([c750de0](https://github.com/NoTIPswe/notip-simulator-backend/commit/c750de09d98b55cb6c781ed34ad6ee6f1e7cd55a))

# [1.2.0](https://github.com/NoTIPswe/notip-simulator-backend/compare/v1.1.0...v1.2.0) (2026-04-08)


### Bug Fixes

* bugs ([de41e8d](https://github.com/NoTIPswe/notip-simulator-backend/commit/de41e8d8bccf596d13b6f7d1a522830cf68d0c17))
* database g serial number ([2522a3c](https://github.com/NoTIPswe/notip-simulator-backend/commit/2522a3c80e06cbbabf89251013167b2e764b3be8))
* refactor sensor management to use ManagementGatewayID instead of GatewayID ([8fd22a8](https://github.com/NoTIPswe/notip-simulator-backend/commit/8fd22a8c81d2331204f48fd555b1a64f086ff0fd))
* simplify error handling in Onboard method and improve test readability ([b25fd80](https://github.com/NoTIPswe/notip-simulator-backend/commit/b25fd8017956a666832715071f98c19ab74728ad))
* stream context gateway ([84dad35](https://github.com/NoTIPswe/notip-simulator-backend/commit/84dad35fa118ee079396503a422f5507c931b28f))
* unify test function naming conventions by removing underscores ([7c14a75](https://github.com/NoTIPswe/notip-simulator-backend/commit/7c14a75457689f716cced0813125cb0d0f2bca8b))


### Features

* add tests for NATS decommission listener and SQLite store functionality ([cb79d07](https://github.com/NoTIPswe/notip-simulator-backend/commit/cb79d07f7df9c0ed6f9d800ec2f3d6bdabb385e7))

# [1.1.0](https://github.com/NoTIPswe/notip-simulator-backend/compare/v1.0.0...v1.1.0) (2026-04-03)


### Bug Fixes

* add integration tests for SQLite store update methods handling non-existent gateways ([1c5fba0](https://github.com/NoTIPswe/notip-simulator-backend/commit/1c5fba042db84391ef2178771c43195b8ad98a5b))
* added integration to .yml ([2424d9c](https://github.com/NoTIPswe/notip-simulator-backend/commit/2424d9cf17a2564a096239ef4a2f317c4df90eb1))
* added missing tests for server.go ([1b7090b](https://github.com/NoTIPswe/notip-simulator-backend/commit/1b7090b3b19971ee90546c740de4dce1938a38cf))
* added tests for coverage ([c96a54f](https://github.com/NoTIPswe/notip-simulator-backend/commit/c96a54fe0dba643bce6db09291d504a5431cd7c1))
* applied sonarqube hints ([8a60819](https://github.com/NoTIPswe/notip-simulator-backend/commit/8a60819fd025b07d86df4991a08ddc45a5cbafb9))
* fixed some files ([7383b03](https://github.com/NoTIPswe/notip-simulator-backend/commit/7383b03fb8162c73395f441a578b139041de2596))
* hopefully last fix in the makefile for coverage ([2382fbd](https://github.com/NoTIPswe/notip-simulator-backend/commit/2382fbd234908801c39aef1bbaa624e60c8354a2))
* last try of the day ([9ce9d02](https://github.com/NoTIPswe/notip-simulator-backend/commit/9ce9d029825c8dec620edba508702b576cc60191))
* maintainability issues highlighted from sonarqube ([1b3063d](https://github.com/NoTIPswe/notip-simulator-backend/commit/1b3063dfd3d3b586c0982aa8ac183e0f75c1a781))
* makefile fixes for coverage ([ee703f4](https://github.com/NoTIPswe/notip-simulator-backend/commit/ee703f4afcd909b95b7055787d69bb7ef3c8e2b8))
* merge conflicts fixed ([4d2bbae](https://github.com/NoTIPswe/notip-simulator-backend/commit/4d2bbaee853c2cedbfd58334d586925506608b7a))
* mismatches in integration tests ([f81e92c](https://github.com/NoTIPswe/notip-simulator-backend/commit/f81e92c4ccfc3b22133ee1fff4e446e7d6cbfd7d))
* modified quality-checks.yml ([ba17d59](https://github.com/NoTIPswe/notip-simulator-backend/commit/ba17d59141fd757f9b8f36ec03e0f8179b9c3579))
* modified server.go and eliminated some useless tests and middleware ([1cf5a76](https://github.com/NoTIPswe/notip-simulator-backend/commit/1cf5a764054ff47a0cfc4520af8c1be562738563))
* modified things for sonarqube ([60f6dc0](https://github.com/NoTIPswe/notip-simulator-backend/commit/60f6dc02098bcb20e4c73ca0e99266f4645c94c5))
* quality-checks.yml modified ([dd441b0](https://github.com/NoTIPswe/notip-simulator-backend/commit/dd441b0bea6a4f2eeb8383006979e4b0c4c71f82))
* refactor provisioning client and gateway handling ([b8e828b](https://github.com/NoTIPswe/notip-simulator-backend/commit/b8e828be58f8220a16ace550fdd823f3a433f69b))
* refactor test function names for consistency and clarity; replace error messages with constants ([8f84ccd](https://github.com/NoTIPswe/notip-simulator-backend/commit/8f84ccda0c39cf9990cc7638790de5edf79ec324))
* solved some issues with github locally ([ec4750d](https://github.com/NoTIPswe/notip-simulator-backend/commit/ec4750ded894dd169c49c2b2ad6c471eb156ed73))
* store.go fixedfor sonarqube ([2bfb93a](https://github.com/NoTIPswe/notip-simulator-backend/commit/2bfb93ac50745b5aca93ab3b301d3003fbe1f637))
* update for securiy hotspots ([f98b8e2](https://github.com/NoTIPswe/notip-simulator-backend/commit/f98b8e20b9e40a0b90d160c2301fef6b915ac8a6))
* various test fixes and adds, api contracts, and gatewaystatus ([44450da](https://github.com/NoTIPswe/notip-simulator-backend/commit/44450da101206897811e41d39531b876cd15485a))


### Features

* add unit tests for clock, gateway, and worker functionalities ([202009e](https://github.com/NoTIPswe/notip-simulator-backend/commit/202009ec2601b4802d5afd79d0f583160b44579a))
* enhance NATS subscriber with manual acknowledgment ([4432964](https://github.com/NoTIPswe/notip-simulator-backend/commit/44329640cf5c4359a839384dda330bcd4b11d54d))
* integration tests and server fixes ([a80afbe](https://github.com/NoTIPswe/notip-simulator-backend/commit/a80afbe3479e2dc8d9aac0119b55e9ac608bb3f8))

# 1.0.0 (2026-03-19)


### Bug Fixes

* added ignore scripts ([bd9571a](https://github.com/NoTIPswe/notip-simulator-backend/commit/bd9571a5d4bd35449c455bf6f900851f673bc1e7))
* added package-lock file ([1342349](https://github.com/NoTIPswe/notip-simulator-backend/commit/1342349c0b4079db26d40b96794a1094099b7133))
* missing working dir ([fda2089](https://github.com/NoTIPswe/notip-simulator-backend/commit/fda2089d007414150139711d20350771a63fb82a))
* release workflow ([64d62da](https://github.com/NoTIPswe/notip-simulator-backend/commit/64d62daab201205ed450113255ef97af21f55044))
* semantic versioning ([7237747](https://github.com/NoTIPswe/notip-simulator-backend/commit/72377479950c63fe06a18a5cf77ef618750adc61))


### Features

* initialize project structure with Go backend, health check endpoint, and CI/CD workflows ([40d9517](https://github.com/NoTIPswe/notip-simulator-backend/commit/40d95178039de27d5e99d9b882afc1f3d08427dd))
