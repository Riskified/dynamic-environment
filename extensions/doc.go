/*
Copyright 2023 Riskified Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// The extension package is a package to aid local customizations of handlers. Every user of this
// operator might have custom needs for operating in the local environment (e.g., node selectors,
// spacial tags, etc.). In order to overload the `Subset` object with all possible customizations, we
// created this package that does not contain any implementations, just empty functions to be called
// from within specific points in the `Reconcile` loop. If a company has spacial requirements, it can
// populate these functions in a local fork of the repository. While it still forces the company
// to handle merges, the merges are in a specific package where chances of merge conflicts are
// reduced to effectively zero.
package extensions
