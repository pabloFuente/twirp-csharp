using System.Net;
using Acme.Clients;
using Acme.Errors;
using Acme.Shop.V1;

namespace Acme.Tests;

/// <summary>
/// The errors of failed calls, as the Twirp specification and its Go client define them.
/// </summary>
public class ErrorTests
{
    private static async Task<AcmeApi.Exception> Fail(HttpResponseMessage response)
    {
        var handler = new StubHandler { Respond = _ => response };
        using var client = Stubs.Client(handler);
        return await Assert.ThrowsAsync<AcmeApi.Exception>(() =>
            AcmeApi.GetItem(client, new GetItemRequest())
        );
    }

    [Fact]
    public async Task TwirpError_KeepsCodeMessageMetaAndStatus()
    {
        var error = await Fail(
            Stubs.Error(
                500,
                "{\"code\":\"dataloss\",\"msg\":\"lost \\\"x\\\" \\u00e9\\n\",\"meta\":{\"k\":\"v\",\"n\":5}}"
            )
        );

        Assert.Equal(AcmeApi.ErrorCode.Data_Loss, error.Type);
        Assert.Equal("lost \"x\" é\n", error.Message);
        Assert.Equal(500, error.Status);
        // Meta values are strings: anything else is left out
        Assert.Equal(new Dictionary<string, string> { ["k"] = "v" }, error.Meta);
    }

    [Fact]
    public async Task TwirpError_ToleratesWhitespaceAndOtherFields()
    {
        var error = await Fail(
            Stubs.Error(
                404,
                "{\n  \"msg\" : \"no item\",\n  \"other\": [1, {\"a\": null}],\n  \"code\" : \"not_found\"\n}\n"
            )
        );

        Assert.Equal(AcmeApi.ErrorCode.Not_Found, error.Type);
        Assert.Equal("no item", error.Message);
        Assert.Empty(error.Meta);
    }

    [Theory]
    [InlineData("canceled", AcmeApi.ErrorCode.Canceled)]
    [InlineData("unknown", AcmeApi.ErrorCode.Unknown)]
    [InlineData("invalid_argument", AcmeApi.ErrorCode.Invalid_Argument)]
    [InlineData("malformed", AcmeApi.ErrorCode.Malformed)]
    [InlineData("deadline_exceeded", AcmeApi.ErrorCode.Deadline_Exceeded)]
    [InlineData("not_found", AcmeApi.ErrorCode.Not_Found)]
    [InlineData("bad_route", AcmeApi.ErrorCode.Bad_Route)]
    [InlineData("already_exists", AcmeApi.ErrorCode.Already_Exists)]
    [InlineData("permission_denied", AcmeApi.ErrorCode.Permission_Denied)]
    [InlineData("unauthenticated", AcmeApi.ErrorCode.Unauthenticated)]
    [InlineData("resource_exhausted", AcmeApi.ErrorCode.Resource_Exhausted)]
    [InlineData("failed_precondition", AcmeApi.ErrorCode.Failed_Precondition)]
    [InlineData("aborted", AcmeApi.ErrorCode.Aborted)]
    [InlineData("out_of_range", AcmeApi.ErrorCode.Out_Of_Range)]
    [InlineData("unimplemented", AcmeApi.ErrorCode.Unimplemented)]
    [InlineData("internal", AcmeApi.ErrorCode.Internal)]
    [InlineData("unavailable", AcmeApi.ErrorCode.Unavailable)]
    [InlineData("dataloss", AcmeApi.ErrorCode.Data_Loss)]
    public async Task EveryTwirpErrorCode_IsRecognized(string code, AcmeApi.ErrorCode expected)
    {
        var error = await Fail(Stubs.Error(500, "{\"code\":\"" + code + "\",\"msg\":\"m\"}"));
        Assert.Equal(expected, error.Type);
    }

    [Fact]
    public async Task UnknownCode_IsAnInternalError()
    {
        var error = await Fail(Stubs.Error(500, "{\"code\":\"nope\",\"msg\":\"m\"}"));

        Assert.Equal(AcmeApi.ErrorCode.Internal, error.Type);
        Assert.Equal("invalid type returned from server error response: nope", error.Message);
        Assert.Equal("{\"code\":\"nope\",\"msg\":\"m\"}", error.Meta["body"]);
    }

    [Theory]
    [InlineData(400, AcmeApi.ErrorCode.Internal)]
    [InlineData(401, AcmeApi.ErrorCode.Unauthenticated)]
    [InlineData(403, AcmeApi.ErrorCode.Permission_Denied)]
    [InlineData(404, AcmeApi.ErrorCode.Bad_Route)]
    [InlineData(429, AcmeApi.ErrorCode.Resource_Exhausted)]
    [InlineData(500, AcmeApi.ErrorCode.Unknown)]
    [InlineData(502, AcmeApi.ErrorCode.Unavailable)]
    [InlineData(503, AcmeApi.ErrorCode.Unavailable)]
    [InlineData(504, AcmeApi.ErrorCode.Unavailable)]
    [InlineData(418, AcmeApi.ErrorCode.Unknown)]
    public async Task ResponseThatIsNotATwirpError_IsMappedFromItsStatus(
        int status,
        AcmeApi.ErrorCode expected
    )
    {
        var error = await Fail(Stubs.Error(status, "<html>proxy error</html>"));

        Assert.Equal(expected, error.Type);
        Assert.Equal(status, error.Status);
        Assert.StartsWith(
            $"Error from intermediary with HTTP status code {status} \"",
            error.Message
        );
        Assert.Equal("true", error.Meta["http_error_from_intermediary"]);
        Assert.Equal(status.ToString(), error.Meta["status_code"]);
        Assert.Equal("<html>proxy error</html>", error.Meta["body"]);
    }

    [Theory]
    [InlineData("")]
    [InlineData("{\"msg\":\"no code\"}")]
    [InlineData("{\"code\":null,\"msg\":\"m\"}")]
    [InlineData("{\"code\":\"\"}")]
    [InlineData("{\"code\":\"internal\"")]
    [InlineData("{\"code\":\"internal\"} trailing")]
    [InlineData("[\"internal\"]")]
    public async Task BodyWithoutATwirpErrorCode_IsAnIntermediaryError(string body)
    {
        var error = await Fail(Stubs.Error(503, body));

        Assert.Equal(AcmeApi.ErrorCode.Unavailable, error.Type);
        Assert.Equal(body, error.Meta["body"]);
    }

    [Fact]
    public async Task Redirect_IsAnInternalIntermediaryError()
    {
        var response = Stubs.Error(302, "");
        response.Headers.Location = new Uri("http://elsewhere/");

        var error = await Fail(response);

        Assert.Equal(AcmeApi.ErrorCode.Internal, error.Type);
        Assert.Equal("http://elsewhere/", error.Meta["location"]);
        Assert.Equal("true", error.Meta["http_error_from_intermediary"]);
        Assert.Contains("Location=\"http://elsewhere/\"", error.Message);
    }

    [Fact]
    public async Task ErrorFactory_BuildsTheException()
    {
        var handler = new StubHandler
        {
            Respond = _ => Stubs.Error(503, "{\"code\":\"unavailable\"}"),
        };
        using var client = Stubs.Client(handler);

        var error = await Assert.ThrowsAsync<CustomException>(() =>
            LegacyApi.GetItem(client, new GetItemRequest())
        );

        Assert.Equal("{\"code\":\"unavailable\"}", error.Body);
        Assert.Equal(503, error.Status);
        Assert.IsAssignableFrom<LegacyApi.Exception>(error);
    }
}
