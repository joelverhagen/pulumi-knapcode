# Knapcode Pulumi Helpers

This Pulumi provider is not for any new cloud resources, per se. It is intended to improve the experience with existing
providers and work around some bugs. Also, I was curious how Pulumi works.

## Installation

Use my [Pulumi.Knapcode](https://www.nuget.org/packages/Pulumi.Knapcode) package on NuGet.org.

```console
dotnet add package Pulumi.Knapcode
```

Then install the resource provider plug-in to your local Pulumi environment:

```console
pulumi plugin install resource knapcode v0.0.1 --server https://github.com/joelverhagen/pulumi-knapcode/releases/download/v0.0.1
```

Note that I only have the Windows x64 build right now.

## Example

This is how you could use the `PrepareAppForWebSignIn` resource.

```csharp
// Declare the app registration
var aadApp = new Pulumi.AzureAD.Application("MyAppRegistration", new ApplicationArgs
{
    DisplayName = "MyAppRegistration"
});

// Declare the website, using the client ID of the app registration
var appService = new Pulumi.Azure.AppService.AppService("MyWebsite", new AppServiceArgs
{
    AppSettings = new InputMap<string>
    {
        { "AzureAd:Instance", "https://login.microsoftonline.com/" },
        { "AzureAd:ClientId", aadApp.ApplicationId },
        { "AzureAd:TenantId", "common" },
    },
});

// Set up the app registration for sign-in using the hostname from the website.
var aadAppUpdate = new Pulumi.Knapcode.PrepareAppForWebSignIn(
    "PrepareAppForWebSignIn",
    new Pulumi.Knapcode.PrepareAppForWebSignInArgs
    {
        ObjectId = aadApp.ObjectId,
        HostName = appService.DefaultSiteHostname,
    });
```

## Providers

This Pulumi provider only has the following resource:

## `knapcode:index:PrepareAppForWebSignIn`

This resource is used to handle several limitations in Pulumi:

- It resolves a circular dependency between Azure AAD app registrations and websites.
- It sets an app registration `signInAudience` to `AzureADandPersonalMicrosoftAccount`.
- It allows deletion of Microsoft Graph app registrations.

### Full explanation

It **resolves a circular dependency** between and an Azure Active Directory app registration used for web sign-in and
the website that is being signed in to. You need the app registration client ID in the website configuration and the
website hostname in the app registration manifest. This is a circular dependency, i.e. a chicken and egg problem.

Pulumi [does not have a fix for circular dependencies](https://github.com/pulumi/pulumi/issues/3021)
but it can be worked around in several ways. [My first attempt](https://github.com/joelverhagen/ExplorePackages/blob/b255c7564059b27eb22d9cd0ec2facf11a6606fd/src/ExplorePackages.Infrastructure/MyStack.cs#L101-L161)
was to execute `az rest` commands from inside of an `Output`.

This resource also **sets app registration `signInAudience` to `AzureADandPersonalMicrosoftAccount`**. This allows
a single app to be used for both Microsoft account sign-in (e.g. @outlook.com) or Azure AD sign-in (e.g. your corporate)
account. This is not possible with Pulumi's AzureAD resource provider because it internally uses the legacy Azure AD
graph APIs (graph.windows.net) which do not support setting this.

If you try to set the `signInAudience` yourself through the legacy Azure AD Graph APIs, you'll get an error like this:
```console
PS> az rest `
    --method PATCH `
    --headers "Content-Type=application/json" `
    --uri https://graph.windows.net/<tenant ID>/applications/<object ID>?api-version=1.6 `
    --body '{\"signInAudience\":\"AzureADandPersonalMicrosoftAccount\"}'

Bad Request({
  "odata.error": {
    "code": "Request_BadRequest",
    "message": {
      "lang": "en",
      "value": "Property 'signInAudience' is read-only and cannot be set."
    },
    "requestId": "<request ID>",
    "date": "<date>"
  }
})
```

This is obviously not true since this property can be set in the Azure Portal or with the newer Microsoft Graph APIs.

If you do not set this sign-in audience and use, say, Microsoft.Identity.Web.UI for your auth flow, you will get an
error like this during sign-in:

`
unauthorized_client: The client does not exist or is not enabled for consumers. If you are the application developer, configure a new application through the App Registrations in the Azure Portal at https://go.microsoft.com/fwlink/?linkid=2083908.
`

So next I updated my `Output` trick to call the new Microsoft Graph API. I ended up doing a request like this to handle
both the chicken and egg problem and the sign-in audience problem:

```console
PS> az rest `
    --method PATCH `
    --headers "Content-Type=application/json" `
    --uri https://graph.microsoft.com/v1.0/applications/<object ID> `
    --body '{\"api\":{\"requestedAccessTokenVersion\":2},' + `
            '\"signInAudience\":\"AzureADandPersonalMicrosoftAccount\",' + `
            '\"web\":{\"homePageUrl\":\"https://<app name>.azurewebsites.net\",' + `
            '\"redirectUris\":[\"https://<app name>.azurewebsites.net/signin-oidc\"],' + `
            '\"logoutUrl\":\"https://<app name>.azurewebsites.net/signout-oidc\"}}'
```

This works great... but it caused *another* problem! I realized that these changes to the app registration manifest
using the Microsoft Graph API put the app registration in a state where the legacy Azure AD graph APIs (graph.windows.net)
could not delete the app registration. This is roughly the request that Pulumi was doing during `pulumi destroy`.

```console
PS> az rest `
    --method DELETE `
    --uri https://graph.windows.net/<tenant ID>/applications/<object ID>?api-version=1.6

Bad Request({
  "odata.error": {
    "code": "Request_BadRequest",
    "message": {
      "lang": "en",
      "value": "Value cannot be null.\r\nParameter name: requestContext"
    },
    "requestId": "<request ID>",
    "date": "<date>"
  }
})
```

The Pulumi error looks like this:
```
Diagnostics:
  azuread:index:Application (<resource name>):
    error: deleting urn:pulumi:<stack>::<project>::azuread:index/application:Application::<resource name>: 1 error occurred:
        * Deleting Application with object ID "<object ID>": graphrbac.ApplicationsClient#Delete: Failure responding to request: StatusCode=400 -- Original Error: autorest/azure: Service returned an error. Status=400 Code="Unknown" Message="Unknown service error" Details=[{"odata.error":{"code":"Request_BadRequest","date":"<date>","message":{"lang":"en","value":"Value cannot be null.\r\nParameter name: requestContext"},"requestId":"<request ID>"}}]
```

So finally it allows the deletion of app registrations that can't be deleted through the legacy Azure AD graph API. It
does this by performing the delete using the Microsoft Graph API. When the Pulumi-based delete occurs, the delete
essentially no-ops with a 404 Not Found since it has already been deleted. It does it with a request like this:

```
PS> az rest `
    --method DELETE `
    --uri https://graph.microsoft.com/v1.0/applications/b30beba6-e1a0-49eb-9473-77adba03253c
```

## Thoughts and discoveries

- The main Pulumi process has both a gRPC server and client which it uses to talk to resource provider plugins.

- It is pretty easy to make your own resource provider. I just copied [pulumi-provider-boilerplate](https://github.com/pulumi/pulumi-provider-boilerplate)
  and fiddled with it until I got it working.

- The best practices for distribution are unclear. I thought NuGet for the SDK and binaries in the GitHub release for
  the Go plug-in was good enough.

- It's not clear to me how to namespace or branch my custom provider. Using the boilerplate, I was unsure how to get the
  SDK-generation tool to NOT prefix my .csproj with "Pulumi." ([source](https://github.com/pulumi/pulumi/blob/5f4e687a1d73b3f806abe47762e95901ae9a6900/pkg/codegen/dotnet/gen.go#L1923)). I'm not thrilled that I have published my NuGet package in the "Pulumi" namespace.

- It seems like too much work to write the Pulumi plug-in in anything but Go. I tried writing it with a C# ASP.NET Core
  gRPC server but it was too much work to work out the flow of what the plug-in should do. I got as far as writing a
  listening gRPC port to STDOUT and importing the .proto files. Then I gave up and decided to learn Go 😂.

- The [Pulumi Slack](https://slack.pulumi.com/) seems full of people having questions but not very many folks answering
  them. It's possible that I was part of that crowd.

## Build 

Simply run `build.ps1` in the root of the repository.
