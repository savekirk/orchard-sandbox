package model

// Orchard response codes, as published at https://docs.anmgw.com/docs-page.html#response-codes.
const (
	CodeNotWhitelisted        = "100"
	CodeNoAuthHeader          = "101"
	CodeInvalidTokens         = "102"
	CodeInvalidSignature      = "103"
	CodeMissingServiceID      = "006"
	CodeMissingCustomerNumber = "008"
	CodeInvalidCustomerNumber = "009"
	CodeMissingAmount         = "010"
	CodeInvalidServiceID      = "011"
	CodeMissingNetwork        = "014"
	CodeAccepted              = "015"
	CodeMissingExttrid        = "018"
	CodeDuplicate             = "021"
	CodeInvalidJSON           = "022"
	CodeInvalidTimestamp      = "023"
	CodeExttridTooLong20      = "025"
	CodeIncompleteParams      = "026"
	CodeCompleted             = "027"
	CodeInvalidAmount         = "028"
	CodeInvalidNetwork        = "029"
	CodeNotFound              = "033"
	CodeInsufficientBalance   = "038"
	CodeMissingCallbackURL    = "039"
	CodeMissingTransType      = "041"
	CodeServiceNotOpen        = "044"
	CodeInvalidCallbackURL    = "045"
	CodeTwoDecimalPlaces      = "046"
	CodeMissingTimestamp      = "053"
	CodeInsufficientAirtime   = "064"
	CodeNoRecord              = "067"
	CodeMissingAccountNumber  = "069"
	CodeAmountTooLow          = "072"
	CodeInsufficientSMS       = "076"
	CodeMissingSenderID       = "077"
	CodeSenderIDTooLong       = "078"
	CodeEmptySMSBody          = "079"
	CodeMissingRecipient      = "081"
	CodeSMSQueued             = "082"
	CodeResendCompleted       = "083"
	CodeExttridTooLong30      = "085"
	CodeInsufficientBillpay   = "086"
	CodeSMSUniqueIDTooLong    = "087"
	CodeMissingBankCode       = "088"
	CodeUndefinedTransType    = "089"
	CodeUndefinedNetwork      = "090"
	CodeUndefinedBankCode     = "091"
)

// Terminal transaction statuses reported in callbacks and status checks.
// Clients read the first three digits: 000 is success, 001 is failure.
const (
	TransStatusSuccess = "000/01"
	TransStatusFailed  = "001/02"
)

var codeDescriptions = map[string]string{
	"100": "You are not allowed to use this service",
	"101": "No Authorization header information",
	"102": "Invalid tokens received",
	"103": "Invalid signature",
	"006": "Missing client service identifier",
	"007": "Missing client token in request",
	"008": "Missing customer number in request",
	"009": "Invalid customer number",
	"010": "Transaction request amount not provided",
	"011": "Invalid service identifier",
	"012": "No IP has been set up for this service",
	"013": "Request could not be successfully completed",
	"014": "Transaction network missing in request",
	"015": "Request successfully received for processing",
	"016": "Transaction reference not provided",
	"018": "Unique transaction identifier missing in request",
	"019": "Account for service does not exist",
	"020": "Request type not provided",
	"021": "Duplicate transaction request",
	"022": "Invalid JSON format",
	"023": "Invalid timestamp in request",
	"024": "Payout mode undefined for service",
	"025": "External transaction id too long. Value must be 20 characters or less",
	"026": "Request parameters not complete",
	"027": "Request successfully completed",
	"028": "Transaction amount invalid",
	"029": "Invalid request network",
	"033": "Transaction not found",
	"036": "Request expired",
	"038": "Insufficient balance",
	"039": "Missing callback url in request",
	"041": "Transaction type not provided",
	"044": "Service not open to this customer",
	"045": "Invalid callback URL provided",
	"046": "Amount must be validated with two decimal places",
	"051": "Unknown request",
	"053": "Missing timestamp in request",
	"055": "Service disabled",
	"064": "Insufficient airtime balance",
	"067": "No record returned",
	"068": "Account name not provided in request",
	"069": "Account number not provided in request",
	"072": "Transaction amount too low",
	"076": "Insufficient balance in sms account",
	"077": "Missing message sender identifier",
	"078": "Message sender identifier too long. Maximum length permitted is nine characters",
	"079": "SMS body is empty",
	"081": "Missing SMS recipient number",
	"082": "Message successfully queued for delivery",
	"083": "Resend request successfully completed",
	"084": "Transaction callback pending",
	"085": "External transaction identifier too long. Maximum length is 30",
	"086": "Insufficient balance in billpay account",
	"087": "SMS unique identifier too long. Maximum length is 40",
	"088": "Missing bank code in request",
	"089": "Undefined transaction type",
	"090": "Undefined transaction network",
	"091": "Undefined bank code in request",
}

// Describe returns Orchard's description for a response code.
func Describe(code string) string {
	if d, ok := codeDescriptions[code]; ok {
		return d
	}
	return "Request could not be successfully completed"
}

// Reply builds the standard {resp_code, resp_desc} body for a code.
func Reply(code string) CommonResponse {
	return CommonResponse{RespCode: code, RespDesc: Describe(code)}
}

// BankCodes lists institutions accepted in bank_code, from Orchard's bank code table.
var BankCodes = map[string]string{
	"RPB": "Republic Bank", "ADB": "ADB", "VOD": "Vodafone Cash", "ZEE": "Zeepay Ghana",
	"ABS": "Absa Bank", "FNB": "FNB", "CBG": "CBG", "GHL": "GHL Bank", "AIR": "AirtelTigo Money",
	"UMB": "UMB", "SIS": "Services Integrity Savings & Loans", "FBN": "FBN Bank", "UBA": "UBA",
	"CAL": "CAL Bank", "SGB": "SG", "ARB": "Apex Bank", "BOG": "Bank of Ghana", "OMN": "Omni Bank",
	"STB": "Stanbic Bank", "FAB": "FAB", "NIB": "NIB", "GMO": "G-Money", "BOA": "BOA",
	"ECO": "Ecobank Ghana", "GTB": "GT Bank", "MTN": "MTN Mobile Money", "FIB": "Fidelity Bank",
	"ZEB": "Zenith Bank", "SCB": "Standard Chartered", "PRB": "PBL", "ACB": "Access Bank", "GCB": "GCB Bank",
}
