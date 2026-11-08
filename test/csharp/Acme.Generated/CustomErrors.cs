namespace Acme.Errors
{
    // The exception that the generated LegacyApi class throws, through its errorFactory option.
    public class CustomException : global::LegacyApi.Exception
    {
        public CustomException(string body, int status)
            : base(global::LegacyApi.ErrorCode.Unknown, "custom error")
        {
            Body = body;
            Status = status;
        }

        public string Body { get; }

        public int Status { get; }
    }

    public static class CustomErrors
    {
        public static CustomException FromResponse(string body, int status)
        {
            return new CustomException(body, status);
        }
    }
}
