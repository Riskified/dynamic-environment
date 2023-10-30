// The extension package is a package to aid local customizations of handlers. Every user of this
// operator might have custom needs for operating in the local environment (e.g. node selectors,
// spacial tags, etc). In order to overload the `Subset` object with all possible customizations we
// created this package that does not contain any implementations, just empty functions to be called
// from within specific points in the `Reconcile` loop. If a company has spacial requirements it can
// populate these functions in a local fork of the repository. While it's still forces the company
// to handle merges, the merges are in a specific package where chances of merge conflicts are
// reduced to effectively zero.
package extensions
